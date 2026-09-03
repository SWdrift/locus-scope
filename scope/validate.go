package scope

import (
	"fmt"
	"sort"
)

func (w *Workspace) validate() error {
	for _, key := range w.sortedScopeKeys() {
		current := w.Scopes[key]
		for _, ref := range current.Manifest.Exports {
			if _, err := w.Resolve(key, ref); err != nil {
				return fmt.Errorf("%s: export %q: %w", current.manifestPath, ref, err)
			}
		}
	}

	for _, key := range w.sortedScopeKeys() {
		current := w.Scopes[key]
		for _, declaration := range current.relationDecls {
			from, err := w.Resolve(key, declaration.from)
			if err != nil {
				return fmt.Errorf("%s:%d: relation %q: start reference %q: %w",
					declaration.source, declaration.line, relationText(declaration), declaration.from, err)
			}
			to, err := w.Resolve(key, declaration.to)
			if err != nil {
				return fmt.Errorf("%s:%d: relation %q: end reference %q: %w",
					declaration.source, declaration.line, relationText(declaration), declaration.to, err)
			}
			w.Relations = append(w.Relations, Relation{From: from, Name: declaration.name, To: to})
		}
	}

	sort.Slice(w.Relations, func(i, j int) bool {
		left, right := w.Relations[i], w.Relations[j]
		if left.From.Scope != right.From.Scope {
			return left.From.Scope < right.From.Scope
		}
		if left.From.ID != right.From.ID {
			return left.From.ID < right.From.ID
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.To.Scope != right.To.Scope {
			return left.To.Scope < right.To.Scope
		}
		return left.To.ID < right.To.ID
	})
	return nil
}

func relationText(declaration relationDecl) string {
	return declaration.from + " " + declaration.name + " " + declaration.to
}
