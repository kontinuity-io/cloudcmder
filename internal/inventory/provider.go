package inventory

import "context"

// Provider is the cloud-backend contract. Add a new cloud by implementing it.
type Provider interface {
	Name() string

	ListScopes(ctx context.Context) ([]Scope, error)

	ListResources(ctx context.Context, s Scope, kinds []Kind) (<-chan ResourceOrErr, error)

	GetDetail(ctx context.Context, ref ResourceRef) (Resource, error)

	// Telemetry returns nil in v1; a non-nil value enables metric overlays in v1.1+.
	Telemetry() Telemetry
}

// Telemetry is the v1.1 metric-overlay extension point. The interface is
// intentionally empty in v1: providers return nil and the TUI hides telemetry
// columns. Future milestones add methods here without touching Provider.
type Telemetry interface{}
