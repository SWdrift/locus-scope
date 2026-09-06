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

	identities := make(map[string]Provenance)
	for _, key := range w.sortedScopeKeys() {
		current := w.Scopes[key]
		for _, declaration := range current.relationDecls {
			from, err := w.Resolve(key, declaration.from)
			if err != nil {
				return fmt.Errorf("%s:%d: relation %q: start reference %q: %w",
					declaration.source.Path, declaration.source.Line, relationText(declaration), declaration.from, err)
			}
			to, err := w.Resolve(key, declaration.to)
			if err != nil {
				return fmt.Errorf("%s:%d: relation %q: end reference %q: %w",
					declaration.source.Path, declaration.source.Line, relationText(declaration), declaration.to, err)
			}
			identity := string(from.Scope) + "\x00" + from.ID + "\x00" + declaration.typ + "\x00" + string(to.Scope) + "\x00" + to.ID
			if previous, duplicate := identities[identity]; duplicate {
				return fmt.Errorf("%s:%d: relation %q duplicates relation declared in %s:%d",
					declaration.source.Path, declaration.source.Line, relationText(declaration), previous.Path, previous.Line)
			}
			identities[identity] = declaration.source
			w.Relations = append(w.Relations, Relation{
				From: from, Type: declaration.typ, To: to,
				FromRef: declaration.from, ToRef: declaration.to,
				Properties: declaration.properties, Source: declaration.source,
			})
		}
	}

	sort.Slice(w.Relations, func(i, j int) bool {
		return CompareRelations(w.Relations[i], w.Relations[j]) < 0
	})
	return nil
}

func relationText(declaration relationDecl) string {
	return declaration.from + " " + declaration.typ + " " + declaration.to
}

// CompareRelations returns a stable ordering for resolved relations.
func CompareRelations(left, right Relation) int {
	values := [][2]string{
		{string(left.From.Scope), string(right.From.Scope)},
		{left.From.ID, right.From.ID},
		{left.Type, right.Type},
		{string(left.To.Scope), string(right.To.Scope)},
		{left.To.ID, right.To.ID},
	}
	for _, pair := range values {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
