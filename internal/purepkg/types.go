package purepkg

import "locus-scope/internal/scope"

// Options controls package resolution and persistence.
type Options struct {
	Registry       string
	FrozenLockfile bool
	Offline        bool
}

// InstallResult describes the committed package environment.
type InstallResult struct {
	Valid     bool           `json:"valid"`
	Root      scope.ScopeKey `json:"root"`
	Added     []string       `json:"added"`
	Removed   []string       `json:"removed"`
	Updated   []string       `json:"updated"`
	Reused    int            `json:"reused"`
	Fetched   int            `json:"fetched"`
	Installed int            `json:"installed"`
	Packages  int            `json:"packages"`
	Scopes    int            `json:"scopes"`
	Entities  int            `json:"entities"`
	Relations int            `json:"relations"`
}

// ListDependency is one importer-relative node in ListResult.
type ListDependency struct {
	Name         string           `json:"name"`
	Version      string           `json:"version"`
	Identity     string           `json:"identity"`
	Dependencies []ListDependency `json:"dependencies"`
}

// ListResult describes the direct dependency trees recorded by the lock.
type ListResult struct {
	Root         string           `json:"root"`
	Dependencies []ListDependency `json:"dependencies"`
}

// PackResult describes a deterministic npm tarball.
type PackResult struct {
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Filename  string   `json:"filename"`
	Integrity string   `json:"integrity"`
	Files     []string `json:"files"`
}

// PublishResult describes a successfully published immutable package version.
type PublishResult struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Registry  string `json:"registry"`
	Integrity string `json:"integrity"`
}
