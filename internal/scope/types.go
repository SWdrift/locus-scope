package scope

// ScopeKey 标识 Workspace 内的一个物理 scope source。
type ScopeKey string

// EntityKey 是 entity 的稳定标识。
type EntityKey struct {
	Scope ScopeKey `json:"scope"`
	ID    string   `json:"id"`
}

// Provenance 标识产生某个对象的声明。
type Provenance struct {
	Scope ScopeKey `json:"scope"`
	File  string   `json:"file"`
	Group string   `json:"group,omitempty"`
	Line  int      `json:"line,omitempty"`
	Index int      `json:"index,omitempty"`
	Path  string   `json:"-"`
}

// Entity 保存规范的 Scope 内 ID 以及对协议不透明的 properties。
type Entity struct {
	ID         string         `json:"id"`
	Properties map[string]any `json:"-"`
	Source     Provenance     `json:"-"`
	LocalID    string         `json:"-"`
}

// Manifest 保存 locus.yaml、locus.yml 或 locus.json 中的 scope 级声明。
type Manifest struct {
	ID      string            `json:"id"`
	Imports map[string]string `json:"imports"`
	Exports []string          `json:"exports"`
}

// Scope 是一个已加载的 source；Key 是其稳定的 source 标识，与 LocalPath 无关。
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

// Relation 是带协议不透明 properties 的已解析有向边。
type Relation struct {
	From       EntityKey      `json:"from"`
	Type       string         `json:"type"`
	To         EntityKey      `json:"to"`
	FromRef    string         `json:"-"`
	ToRef      string         `json:"-"`
	Properties map[string]any `json:"-"`
	Source     Provenance     `json:"-"`
}

// Workspace 包含从 Root 可达的完整 graph。
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
