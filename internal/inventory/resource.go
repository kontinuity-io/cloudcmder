package inventory

// ResourceRef is the canonical cross-provider identifier for any resource.
type ResourceRef struct {
	Provider string
	ScopeID  string
	Kind     Kind
	ID       string
}

// String returns the canonical "provider:scope:Kind:id" form used as the
// primary key in the SQLite store and as the edge endpoint in the graph.
func (r ResourceRef) String() string {
	return r.Provider + ":" + r.ScopeID + ":" + string(r.Kind) + ":" + r.ID
}

// Scope is a provider-native container for resources (GCP project, AWS
// account/region, Azure subscription). Marshaled to JSON for `--list-scopes`.
type Scope struct {
	ProviderID  string            `json:"providerId"`
	ID          string            `json:"id"`
	DisplayName string            `json:"displayName"`
	Parent      string            `json:"parent,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// Resource is the normalized form every provider produces and every consumer
// (store, TUI, exporter) reads. Detail and Native are the two intentional
// any-typed escape hatches; all other fields are strongly typed.
type Resource struct {
	Ref    ResourceRef
	Kind   Kind
	Name   string
	Region string
	Status string
	Labels map[string]string
	Detail any
	Refs   map[RefKind][]ResourceRef
	Native any
}

// ResourceOrErr is the per-element type sent over the ListResources channel.
// A non-nil Err is a non-fatal issue (e.g., one resource failed enrichment);
// fatal errors come back via the second return of ListResources itself.
type ResourceOrErr struct {
	Resource Resource
	Err      error
}
