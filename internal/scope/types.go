package scope

// ScopeKey identifies one physical scope source within a workspace.
type ScopeKey string

// EntityKey is the stable identity of an entity.
type EntityKey struct {
	Scope ScopeKey `json:"scope"`
	ID    string   `json:"id"`
}

// Provenance identifies the declaration that produced an object.
type Provenance struct {
	Scope ScopeKey `json:"scope"`
	File  string   `json:"file"`
	Group string   `json:"group,omitempty"`
	Line  int      `json:"line,omitempty"`
	Index int      `json:"index,omitempty"`
	Path  string   `json:"-"`
}

// Entity stores a canonical Scope-local ID and protocol-opaque properties.
type Entity struct {
	ID         string         `json:"id"`
	Properties map[string]any `json:"-"`
	Source     Provenance     `json:"-"`
	LocalID    string         `json:"-"`
}

// Manifest contains the scope-level declarations from locus.yaml, locus.yml, or locus.json.
type Manifest struct {
	ID      string            `json:"id"`
	Imports map[string]string `json:"imports"`
	Exports []string          `json:"exports"`
}

// Scope is one loaded source. Key is its stable source identity, independent of LocalPath.
type Scope struct {
	Key          ScopeKey            `json:"key"`
	Manifest     Manifest            `json:"manifest"`
	Entities     map[string]Entity   `json:"entities"`
	Imports      map[string]ScopeKey `json:"imports"`
	LocalPath    string              `json:"-"`
	ManifestFile string              `json:"-"`

	manifestPath  string
	exported      map[string]struct{}
	relationDecls []relationDecl
}

// Relation is a resolved directed edge with protocol-opaque properties.
type Relation struct {
	From       EntityKey      `json:"from"`
	Type       string         `json:"type"`
	To         EntityKey      `json:"to"`
	FromRef    string         `json:"-"`
	ToRef      string         `json:"-"`
	Properties map[string]any `json:"-"`
	Source     Provenance     `json:"-"`
}

// Workspace contains the complete graph reachable from Root.
type Workspace struct {
	Root      ScopeKey            `json:"root"`
	Scopes    map[ScopeKey]*Scope `json:"scopes"`
	Relations []Relation          `json:"relations"`
}

type relationDecl struct {
	from       string
	typ        string
	to         string
	properties map[string]any
	source     Provenance
}
