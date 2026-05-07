package gcp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	functions "cloud.google.com/go/functions/apiv2"
	"cloud.google.com/go/functions/apiv2/functionspb"
	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"cloudcmder.com/internal/inventory"
)

// --- Cloud Run Services ---------------------------------------------------

type runServicesAPI interface {
	List(ctx context.Context, parent string) runServicesIterator
	Close() error
}

type runServicesIterator interface {
	Next() (*runpb.Service, error)
}

type realRunClient struct {
	c *run.ServicesClient
}

func (r *realRunClient) List(ctx context.Context, parent string) runServicesIterator {
	return r.c.ListServices(ctx, &runpb.ListServicesRequest{Parent: parent})
}

func (r *realRunClient) Close() error { return r.c.Close() }

type runFactory func(ctx context.Context, opts ...option.ClientOption) (runServicesAPI, error)

type runClientState struct {
	once    sync.Once
	cli     runServicesAPI
	err     error
	factory runFactory
}

func (p *GCPProvider) runClient(ctx context.Context) (runServicesAPI, error) {
	p.runsvc.once.Do(func() {
		if p.runsvc.factory != nil {
			p.runsvc.cli, p.runsvc.err = p.runsvc.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.runsvc.err = fmt.Errorf("gcp: ADC for run client: %w", err)
			return
		}
		c, err := run.NewServicesClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.runsvc.err = fmt.Errorf("gcp: new run client: %w", err)
			return
		}
		p.runsvc.cli = &realRunClient{c: c}
	})
	if p.runsvc.err != nil {
		return nil, p.runsvc.err
	}
	return p.runsvc.cli, nil
}

func (p *GCPProvider) closeRunClient() error {
	if p.runsvc.cli == nil {
		return nil
	}
	return p.runsvc.cli.Close()
}

// --- Cloud Functions Gen2 -------------------------------------------------

type functionsAPI interface {
	List(ctx context.Context, parent string) functionsIterator
	Close() error
}

type functionsIterator interface {
	Next() (*functionspb.Function, error)
}

type realFunctionsClient struct {
	c *functions.FunctionClient
}

func (r *realFunctionsClient) List(ctx context.Context, parent string) functionsIterator {
	return r.c.ListFunctions(ctx, &functionspb.ListFunctionsRequest{Parent: parent})
}

func (r *realFunctionsClient) Close() error { return r.c.Close() }

type functionsFactory func(ctx context.Context, opts ...option.ClientOption) (functionsAPI, error)

type functionsClientState struct {
	once    sync.Once
	cli     functionsAPI
	err     error
	factory functionsFactory
}

func (p *GCPProvider) functionsClient(ctx context.Context) (functionsAPI, error) {
	p.funcs.once.Do(func() {
		if p.funcs.factory != nil {
			p.funcs.cli, p.funcs.err = p.funcs.factory(ctx)
			return
		}
		creds, err := NewCredentials(ctx)
		if err != nil {
			p.funcs.err = fmt.Errorf("gcp: ADC for functions client: %w", err)
			return
		}
		c, err := functions.NewFunctionClient(ctx, option.WithCredentials(creds))
		if err != nil {
			p.funcs.err = fmt.Errorf("gcp: new functions client: %w", err)
			return
		}
		p.funcs.cli = &realFunctionsClient{c: c}
	})
	if p.funcs.err != nil {
		return nil, p.funcs.err
	}
	return p.funcs.cli, nil
}

func (p *GCPProvider) closeFunctionsClient() error {
	if p.funcs.cli == nil {
		return nil
	}
	return p.funcs.cli.Close()
}

// --- Enrichment ------------------------------------------------------------

// enrichFunctions emits FunctionDetail rows for both Cloud Run Services and
// Cloud Functions Gen2 — both back onto the same KindFunction per the asset
// type mapping. Errors from either side surface but don't abort the other.
func enrichFunctions(ctx context.Context, p *GCPProvider, scope inventory.Scope, ch chan<- inventory.ResourceOrErr) {
	parent := "projects/" + scope.ID + "/locations/-"

	if rc, err := p.runClient(ctx); err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: run client: %w", err)})
	} else {
		it := rc.List(ctx, parent)
		for {
			s, err := it.Next()
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list run services: %w", err)})
				break
			}
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildRunServiceResource(scope.ID, s, p.dumpNative)})
		}
	}
	if ctx.Err() != nil {
		return
	}

	if fc, err := p.functionsClient(ctx); err != nil {
		sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: functions client: %w", err)})
	} else {
		it := fc.List(ctx, parent)
		for {
			f, err := it.Next()
			if errors.Is(err, iterator.Done) {
				return
			}
			if err != nil {
				sendOrCancel(ctx, ch, inventory.ResourceOrErr{Err: fmt.Errorf("gcp: list cloud functions: %w", err)})
				return
			}
			sendOrCancel(ctx, ch, inventory.ResourceOrErr{Resource: buildCloudFunctionResource(scope.ID, f, p.dumpNative)})
		}
	}
}

func buildRunServiceResource(scopeID string, s *runpb.Service, dumpNative bool) inventory.Resource {
	name := lastSegment(s.GetName())
	region := regionFromResourceName(s.GetName())

	detail := inventory.FunctionDetail{
		Trigger: "HTTP",
		Region:  region,
	}
	if tmpl := s.GetTemplate(); tmpl != nil {
		if scaling := tmpl.GetScaling(); scaling != nil {
			detail.MaxInst = scaling.GetMaxInstanceCount()
		}
		if cs := tmpl.GetContainers(); len(cs) > 0 {
			detail.Runtime = lastSegment(cs[0].GetImage())
			if res := cs[0].GetResources(); res != nil {
				if mem, ok := res.GetLimits()["memory"]; ok {
					detail.MemoryMiB = parseMemory(mem)
				}
				if cpu, ok := res.GetLimits()["cpu"]; ok {
					detail.CPUs = parseCPUs(cpu)
				}
			}
		}
	}

	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindFunction, ID: name},
		Kind:   inventory.KindFunction,
		Name:   name,
		Region: region,
		Status: "RUNNING",
		Labels: s.GetLabels(),
		Detail: &detail,
		Native: nativeFrom(dumpNative, s),
	}
}

func buildCloudFunctionResource(scopeID string, f *functionspb.Function, dumpNative bool) inventory.Resource {
	name := lastSegment(f.GetName())
	region := regionFromResourceName(f.GetName())

	detail := inventory.FunctionDetail{
		Region: region,
	}
	if bc := f.GetBuildConfig(); bc != nil {
		detail.Runtime = bc.GetRuntime()
	}
	if sc := f.GetServiceConfig(); sc != nil {
		detail.MemoryMiB = parseMemory(sc.GetAvailableMemory())
		detail.CPUs = parseCPUs(sc.GetAvailableCpu())
		detail.MaxInst = sc.GetMaxInstanceCount()
	}
	if et := f.GetEventTrigger(); et != nil {
		detail.Trigger = et.GetEventType()
	} else {
		detail.Trigger = "HTTP"
	}

	return inventory.Resource{
		Ref:    inventory.ResourceRef{Provider: providerName, ScopeID: scopeID, Kind: inventory.KindFunction, ID: name},
		Kind:   inventory.KindFunction,
		Name:   name,
		Region: region,
		Status: f.GetState().String(),
		Labels: f.GetLabels(),
		Detail: &detail,
		Native: nativeFrom(dumpNative, f),
	}
}

// regionFromResourceName extracts the location component from a GCP resource
// path like "projects/p/locations/us-central1/services/foo" → "us-central1".
func regionFromResourceName(name string) string {
	parts := strings.Split(name, "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "locations" {
			return parts[i+1]
		}
	}
	return ""
}

// parseMemory turns "512Mi" / "1Gi" / "256" into MiB.
func parseMemory(s string) int64 {
	if s == "" {
		return 0
	}
	upper := strings.ToUpper(s)
	switch {
	case strings.HasSuffix(upper, "GI"):
		v, _ := strconv.ParseInt(upper[:len(upper)-2], 10, 64)
		return v * 1024
	case strings.HasSuffix(upper, "MI"):
		v, _ := strconv.ParseInt(upper[:len(upper)-2], 10, 64)
		return v
	case strings.HasSuffix(upper, "G"):
		v, _ := strconv.ParseInt(upper[:len(upper)-1], 10, 64)
		return v * 1024
	case strings.HasSuffix(upper, "M"):
		v, _ := strconv.ParseInt(upper[:len(upper)-1], 10, 64)
		return v
	default:
		v, _ := strconv.ParseInt(upper, 10, 64)
		return v / (1024 * 1024)
	}
}

// parseCPUs handles the "1000m" Kubernetes-style millicpu string and bare floats.
func parseCPUs(s string) float64 {
	if s == "" {
		return 0
	}
	if strings.HasSuffix(s, "m") {
		v, _ := strconv.ParseFloat(s[:len(s)-1], 64)
		return v / 1000.0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
