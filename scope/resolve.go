package scope

import (
	"fmt"
	"strings"
)

// Resolve resolves ref from the named scope while enforcing every projection's
// export boundary. The returned key always names the entity's owning scope.
func (w *Workspace) Resolve(from ScopeKey, ref string) (EntityKey, error) {
	current, ok := w.Scopes[from]
	if !ok {
		return EntityKey{}, fmt.Errorf("scope source %q is not loaded", from)
	}
	if strings.TrimSpace(ref) == "" {
		return EntityKey{}, fmt.Errorf("%s: entity reference is empty", scopeLabel(current))
	}

	projection, rest, projected := strings.Cut(ref, ":")
	if !projected {
		if _, exists := current.Entities[ref]; !exists {
			return EntityKey{}, fmt.Errorf("%s does not contain entity %q", scopeLabel(current), ref)
		}
		return EntityKey{Scope: current.Key, ID: ref}, nil
	}
	if projection == "" || rest == "" {
		return EntityKey{}, fmt.Errorf("%s: malformed projected reference %q", scopeLabel(current), ref)
	}

	targetKey, imported := current.Imports[projection]
	if !imported {
		return EntityKey{}, fmt.Errorf("%s does not import projection %q referenced by %q", scopeLabel(current), projection, ref)
	}
	target := w.Scopes[targetKey]
	if target == nil {
		return EntityKey{}, fmt.Errorf("%s: projection %q targets unloaded scope %q", scopeLabel(current), projection, targetKey)
	}
	if _, visible := target.exported[rest]; !visible {
		return EntityKey{}, fmt.Errorf("%s does not export %q (required by projection %q from %s)", scopeLabel(target), rest, projection, scopeLabel(current))
	}
	return w.Resolve(targetKey, rest)
}

func scopeLabel(s *Scope) string {
	return fmt.Sprintf("scope %q (%s)", s.Manifest.ID, s.Key)
}
