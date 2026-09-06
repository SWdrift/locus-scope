package pkg

import (
	"fmt"

	"locus-scope/internal/packageenv"
	"locus-scope/internal/scope"
)

func loadEnvironment(root string, lock lockFile, materialized map[string]materializedPackage) (*scope.Workspace, error) {
	environment := packageenv.Environment{
		RootDependencies: map[string]packageenv.Identity{},
		Packages:         map[packageenv.Identity]packageenv.Package{},
	}
	rootImporter := lock.Importers["."]
	for name, edge := range rootImporter.Dependencies {
		if installed, ok := materialized[edge.Package]; ok && installed.isLocus {
			environment.RootDependencies[name] = packageenv.Identity(edge.Package)
		}
	}
	for identity, installed := range materialized {
		if !installed.isLocus {
			continue
		}
		dependencies := map[string]packageenv.Identity{}
		for name, edge := range lock.Packages[identity].Dependencies {
			if target, ok := materialized[edge.Package]; ok && target.isLocus {
				dependencies[name] = packageenv.Identity(edge.Package)
			}
		}
		environment.Packages[packageenv.Identity(identity)] = packageenv.Package{
			Identity:     packageenv.Identity(identity),
			Root:         installed.root,
			Entry:        installed.metadata.Entry,
			Dependencies: dependencies,
		}
	}
	workspace, err := packageenv.Load(root, environment)
	if err != nil {
		return nil, fmt.Errorf("load package environment: %w", err)
	}
	return workspace, nil
}

func workspaceCounts(workspace *scope.Workspace) (scopes, entities, relations int) {
	scopes = len(workspace.Scopes)
	for _, loadedScope := range workspace.Scopes {
		entities += len(loadedScope.Entities)
	}
	return scopes, entities, len(workspace.Relations)
}
