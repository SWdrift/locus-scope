package scope

// ScopeKey identifies one physical scope source within a workspace.
type ScopeKey string

// EntityKey is the stable identity of an entity.
type EntityKey struct {
	Scope ScopeKey `json:"scope"`
	ID    string   `json:"id"`
}

// Entity stores an ID and protocol-opaque properties.
type Entity struct {
	ID         string         `json:"id"`
	Properties map[string]any `json:"properties"`
}

// Manifest contains the scope-level declarations from locus.yaml, locus.yml, or locus.json.
type Manifest struct {
	ID      string            `json:"id"`
	Imports map[string]string `json:"imports"`
	Exports []string          `json:"exports"`
}

// Scope is one loaded local scope. Key is its canonical source directory.
type Scope struct {
	Key      ScopeKey            `json:"key"`
	Manifest Manifest            `json:"manifest"`
	Entities map[string]Entity   `json:"entities"`
	Imports  map[string]ScopeKey `json:"imports"`

	manifestPath  string
	exported      map[string]struct{}
	entityOrigins map[string]string
	relationDecls []relationDecl
}

// Relation is a resolved directed edge between stable entity identities.
type Relation struct {
	From EntityKey `json:"from"`
	Name string    `json:"name"`
	To   EntityKey `json:"to"`
}

// Workspace contains the complete graph reachable from Root.
type Workspace struct {
	Root      ScopeKey            `json:"root"`
	Scopes    map[ScopeKey]*Scope `json:"scopes"`
	Relations []Relation          `json:"relations"`
}

type relationDecl struct {
	from   string
	name   string
	to     string
	source string
	line   int
}
