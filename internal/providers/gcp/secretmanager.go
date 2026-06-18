package gcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// smSecret is the provider-internal projection of one Secret Manager secret
// plus its derived version/rotation/monitoring facts. It is the seam the
// build/enrich logic is tested against — the real client (untested, like every
// other realXClient) translates the Secret Manager SDK types into this shape.
type smSecret struct {
	ID               string // secret short name (last path segment) = CAI stub id
	Replication      string // "automatic" | "user-managed"
	Region           string // "" for automatic; first replica location for user-managed
	ActiveVersions   int    // count of ENABLED versions
	RotationPeriod   string // human duration, e.g. "30d"; empty if no rotation
	RotationTopic    string // first Pub/Sub topic name (short); empty if none
	AccessOperations int64  // access-version ops over the metric window (best-effort)
}

// secretManagerAPI is the seam between enrichSecretManager and Cloud Secret
// Manager. Tests inject a fake; production uses realSecretManagerClient.
type secretManagerAPI interface {
	ListSecrets(ctx context.Context, projectID string) ([]smSecret, error)
	Close() error
}

// realSecretManagerClient holds the credential options rather than a live
// client: secretmanager.NewClient is project-agnostic but we still build a
// fresh client per List call to keep the --scan-all (one provider, many
// projects) story identical to the other realXClients.
type realSecretManagerClient struct {
	opts []option.ClientOption
}

func (r *realSecretManagerClient) ListSecrets(ctx context.Context, projectID string) ([]smSecret, error) {
	c, err := secretmanager.NewClient(ctx, r.opts...)
	if err != nil {
		return nil, fmt.Errorf("new secretmanager client: %w", err)
	}
	defer func() { _ = c.Close() }()

	// access_count metrics for the whole project in one best-effort call;
	// missing entries fall back to zero so a disabled Monitoring API (or a
	// missing permission) never aborts the secret scan.
	accessByID := r.loadAccessMetrics(ctx, projectID)

	var out []smSecret
	it := c.ListSecrets(ctx, &secretmanagerpb.ListSecretsRequest{
		Parent: "projects/" + projectID,
	})
	for {
		s, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list secrets: %w", err)
		}
		sec := smSecret{ID: lastSegment(s.GetName())}
		sec.Replication, sec.Region = secretReplication(s.GetReplication())
		sec.RotationPeriod = secretRotationPeriod(s.GetRotation())
		sec.RotationTopic = firstTopic(s.GetTopics())
		sec.ActiveVersions = r.countEnabledVersions(ctx, c, s.GetName())
		sec.AccessOperations = accessByID[sec.ID]
		out = append(out, sec)
	}
	return out, nil
}

// countEnabledVersions walks ListSecretVersions for one secret and counts the
// ENABLED ones. Best-effort: an unreadable version list contributes 0 rather
// than aborting the whole secret scan.
func (r *realSecretManagerClient) countEnabledVersions(ctx context.Context, c *secretmanager.Client, secretName string) int {
	it := c.ListSecretVersions(ctx, &secretmanagerpb.ListSecretVersionsRequest{
		Parent: secretName,
	})
	var n int
	for {
		v, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			break
		}
		if v.GetState() == secretmanagerpb.SecretVersion_ENABLED {
			n++
		}
	}
	return n
}

// loadAccessMetrics returns access-operation counts keyed by secret short name.
// Best-effort via Cloud Monitoring: any failure (API disabled, missing
// permission, no samples yet) yields an empty map so affected secrets show 0.
func (r *realSecretManagerClient) loadAccessMetrics(ctx context.Context, projectID string) map[string]int64 {
	mc, err := monitoringMetricClient(ctx, r.opts)
	if err != nil {
		slog.Warn("scan: monitoring client unavailable; secret access ops = 0",
			"project", projectID, "error", err)
		return nil
	}
	defer func() { _ = mc.Close() }()
	m, err := listSecretAccessMetrics(ctx, mc, projectID)
	if err != nil {
		slog.Warn("scan: secret access metrics unavailable; access ops = 0",
			"project", projectID, "error", err)
		return nil
	}
	return m
}

func (r *realSecretManagerClient) Close() error { return nil }

// secretReplication classifies the replication policy and returns the region
// for user-managed replication (first replica's location). Automatic
// (global) replication has no region.
func secretReplication(repl *secretmanagerpb.Replication) (policy, region string) {
	if repl == nil {
		return "", ""
	}
	if um := repl.GetUserManaged(); um != nil {
		region = firstReplicaLocation(um.GetReplicas())
		return "user-managed", region
	}
	if repl.GetAutomatic() != nil {
		return "automatic", ""
	}
	return "", ""
}

func firstReplicaLocation(replicas []*secretmanagerpb.Replication_UserManaged_Replica) string {
	for _, rp := range replicas {
		if loc := rp.GetLocation(); loc != "" {
			return loc
		}
	}
	return ""
}

// secretRotationPeriod renders the rotation period as a human duration
// (e.g. "30d"). Empty when no rotation policy or no period is set.
func secretRotationPeriod(rot *secretmanagerpb.Rotation) string {
	if rot == nil {
		return ""
	}
	p := rot.GetRotationPeriod()
	if p == nil {
		return ""
	}
	return humanDuration(p.AsDuration())
}

// firstTopic returns the short name of the first configured rotation Pub/Sub
// topic (last path segment of projects/*/topics/*). Empty when none.
func firstTopic(topics []*secretmanagerpb.Topic) string {
	for _, t := range topics {
		if n := t.GetName(); n != "" {
			return lastSegment(n)
		}
	}
	return ""
}

type secretManagerFactory func(ctx context.Context, opts ...option.ClientOption) (secretManagerAPI, error)

type secretManagerClientState struct {
	once    sync.Once
	cli     secretManagerAPI
	err     error
	factory secretManagerFactory
}

func (p *GCPProvider) secretManagerClient(ctx context.Context) (secretManagerAPI, error) {
	p.secretManager.once.Do(func() {
		if p.secretManager.factory != nil {
			p.secretManager.cli, p.secretManager.err = p.secretManager.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.secretManager.err = fmt.Errorf("gcp: ADC for secretmanager client: %w", err)
			return
		}
		p.secretManager.cli = &realSecretManagerClient{opts: []option.ClientOption{option.WithCredentials(creds)}}
	})
	if p.secretManager.err != nil {
		return nil, p.secretManager.err
	}
	return p.secretManager.cli, nil
}

func (p *GCPProvider) closeSecretManagerClient() error {
	if p.secretManager.cli == nil {
		return nil
	}
	return p.secretManager.cli.Close()
}

// enrichSecretManager emits SecretManagerDetail rows at the secret grain. These
// overwrite the CAI Phase-1 stub rows (matching Ref ID = secret short name).
func enrichSecretManager(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	sc, err := p.secretManagerClient(ctx)
	if err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: secretmanager client: %w", err)})
		return
	}
	secrets, err := sc.ListSecrets(ctx, scope.ID)
	if err != nil {
		// A disabled Secret Manager API or a missing list permission is a
		// per-kind failure, not a scan failure: keep the CAI Phase-1 stub
		// rows and surface the issue as a warning (architecture.md §"Error
		// Handling").
		if IsRecoverableScanErr(err) {
			slog.Warn("scan: secret manager unavailable; keeping stub rows",
				"project", scope.ID, "error", err)
			return
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list secrets: %w", err)})
		return
	}
	for _, s := range secrets {
		if ctx.Err() != nil {
			return
		}
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildSecretManagerResource(scope.ID, s)})
	}
}

func buildSecretManagerResource(scopeID string, s smSecret) inventory.Resource {
	detail := inventory.SecretManagerDetail{
		Subtype:          "Secret",
		Region:           s.Region,
		ActiveVersions:   s.ActiveVersions,
		Replication:      s.Replication,
		RotationPeriod:   s.RotationPeriod,
		RotationTopic:    s.RotationTopic,
		AccessOperations: s.AccessOperations,
	}
	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindGCPSecretManager, ID: s.ID},
		Kind:   inventory.KindGCPSecretManager,
		Name:   s.ID,
		Region: s.Region,
		Detail: &detail,
	}
}
