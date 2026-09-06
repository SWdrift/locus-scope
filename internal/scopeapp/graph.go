package scopeapp

import (
	"fmt"
	"sort"

	"gonum.org/v1/gonum/graph/multi"
	"locus-scope/internal/scope"
)

type GraphResult struct {
	Seeds     []Entity   `json:"seeds"`
	Depth     int        `json:"depth"`
	Nodes     []Entity   `json:"nodes"`
	Relations []Relation `json:"relations"`
}

type PathResult struct {
	From      Entity     `json:"from"`
	To        Entity     `json:"to"`
	Nodes     []Entity   `json:"nodes"`
	Relations []Relation `json:"relations"`
}

type ImpactResult struct {
	Seeds     []Entity   `json:"seeds"`
	Affected  []Entity   `json:"affected"`
	Relations []Relation `json:"relations"`
}

type graphAdapter struct {
	graph *multi.DirectedGraph
	nodes map[scope.EntityKey]int64
}

func newGraphAdapter(workspace *scope.Workspace) *graphAdapter {
	adapter := &graphAdapter{graph: multi.NewDirectedGraph(), nodes: make(map[scope.EntityKey]int64)}
	keys := make([]scope.EntityKey, 0)
	for owner, current := range workspace.Scopes {
		for id := range current.Entities {
			keys = append(keys, scope.EntityKey{Scope: owner, ID: id})
		}
	}
	sortEntityKeys(keys)
	for _, key := range keys {
		node := adapter.graph.NewNode()
		adapter.graph.AddNode(node)
		adapter.nodes[key] = node.ID()
	}
	for _, relation := range workspace.Relations {
		from, to := adapter.graph.Node(adapter.nodes[relation.From]), adapter.graph.Node(adapter.nodes[relation.To])
		adapter.graph.SetLine(adapter.graph.NewLine(from, to))
	}
	return adapter
}

func (s *Service) Graph(seeds []scope.EntityKey, depth int, via []scope.Predicate, withSource bool) (GraphResult, error) {
	if depth < 0 {
		return GraphResult{}, fmt.Errorf("graph depth must be non-negative")
	}
	adapter := newGraphAdapter(s.workspace)
	levels := make(map[scope.EntityKey]int)
	queue := make([]scope.EntityKey, 0, len(seeds))
	for _, seed := range seeds {
		if _, err := s.EntityByKey(seed, false); err != nil {
			return GraphResult{}, err
		}
		if _, exists := levels[seed]; !exists {
			levels[seed] = 0
			queue = append(queue, seed)
		}
	}
	selected := make(map[int]struct{})
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if levels[current] >= depth {
			continue
		}
		for index, relation := range s.workspace.Relations {
			if relation.From != current || !s.matchRelation(relation, via) {
				continue
			}
			if !adapter.graph.HasEdgeFromTo(adapter.nodes[relation.From], adapter.nodes[relation.To]) {
				continue
			}
			selected[index] = struct{}{}
			if _, exists := levels[relation.To]; !exists {
				levels[relation.To] = levels[current] + 1
				queue = append(queue, relation.To)
			}
		}
	}
	return s.graphResult(seeds, depth, levels, selected, withSource)
}

func (s *Service) Path(from, to scope.EntityKey, via []scope.Predicate, withSource bool) (PathResult, error) {
	if _, err := s.EntityByKey(from, false); err != nil {
		return PathResult{}, err
	}
	if _, err := s.EntityByKey(to, false); err != nil {
		return PathResult{}, err
	}
	newGraphAdapter(s.workspace)
	previous := make(map[scope.EntityKey]scope.EntityKey)
	previousRelation := make(map[scope.EntityKey]int)
	seen := map[scope.EntityKey]struct{}{from: {}}
	queue := []scope.EntityKey{from}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if current == to {
			break
		}
		for index, relation := range s.workspace.Relations {
			if relation.From != current || !s.matchRelation(relation, via) {
				continue
			}
			if _, exists := seen[relation.To]; exists {
				continue
			}
			seen[relation.To] = struct{}{}
			previous[relation.To] = current
			previousRelation[relation.To] = index
			queue = append(queue, relation.To)
		}
	}
	if _, found := seen[to]; !found {
		return PathResult{}, fmt.Errorf("no directed path from %s#%s to %s#%s", from.Scope, from.ID, to.Scope, to.ID)
	}
	keys := []scope.EntityKey{to}
	relationIndexes := make([]int, 0)
	for current := to; current != from; {
		relationIndexes = append(relationIndexes, previousRelation[current])
		current = previous[current]
		keys = append(keys, current)
	}
	reverseEntityKeys(keys)
	reverseInts(relationIndexes)
	nodes, err := s.entitiesByKeys(keys, withSource)
	if err != nil {
		return PathResult{}, err
	}
	relations := s.relationsByIndexes(relationIndexes, withSource)
	return PathResult{From: nodes[0], To: nodes[len(nodes)-1], Nodes: nodes, Relations: relations}, nil
}

func (s *Service) Impact(seeds []scope.EntityKey, via []scope.Predicate, withSource bool) (ImpactResult, error) {
	seen := make(map[scope.EntityKey]struct{})
	seedSet := make(map[scope.EntityKey]struct{})
	queue := make([]scope.EntityKey, 0, len(seeds))
	for _, seed := range seeds {
		if _, err := s.EntityByKey(seed, false); err != nil {
			return ImpactResult{}, err
		}
		if _, ok := seen[seed]; !ok {
			seen[seed] = struct{}{}
			seedSet[seed] = struct{}{}
			queue = append(queue, seed)
		}
	}
	selected := make(map[int]struct{})
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for index, relation := range s.workspace.Relations {
			if relation.To != current || !s.matchRelation(relation, via) {
				continue
			}
			selected[index] = struct{}{}
			if _, ok := seen[relation.From]; !ok {
				seen[relation.From] = struct{}{}
				queue = append(queue, relation.From)
			}
		}
	}
	seedItems, _ := s.entitiesByKeys(sortedKeySet(seedSet), withSource)
	affectedSet := make(map[scope.EntityKey]struct{}, len(seen))
	for key := range seen {
		if _, seed := seedSet[key]; !seed {
			affectedSet[key] = struct{}{}
		}
	}
	affected, _ := s.entitiesByKeys(sortedKeySet(affectedSet), withSource)
	return ImpactResult{Seeds: seedItems, Affected: affected, Relations: s.relationsBySet(selected, withSource)}, nil
}

func (s *Service) graphResult(seeds []scope.EntityKey, depth int, levels map[scope.EntityKey]int, selected map[int]struct{}, withSource bool) (GraphResult, error) {
	seedSet := make(map[scope.EntityKey]struct{})
	for _, key := range seeds {
		seedSet[key] = struct{}{}
	}
	seedItems, err := s.entitiesByKeys(sortedKeySet(seedSet), withSource)
	if err != nil {
		return GraphResult{}, err
	}
	nodeSet := make(map[scope.EntityKey]struct{}, len(levels))
	for key := range levels {
		nodeSet[key] = struct{}{}
	}
	nodes, err := s.entitiesByKeys(sortedKeySet(nodeSet), withSource)
	if err != nil {
		return GraphResult{}, err
	}
	return GraphResult{Seeds: seedItems, Depth: depth, Nodes: nodes, Relations: s.relationsBySet(selected, withSource)}, nil
}

func (s *Service) matchRelation(relation scope.Relation, predicates []scope.Predicate) bool {
	object := cloneMap(relation.Properties)
	object["from"], object["type"], object["to"] = relation.FromRef, relation.Type, relation.ToRef
	metadata := map[string]any{"scope": s.scopeRefs[relation.Source.Scope], "fromScope": s.scopeRefs[relation.From.Scope], "toScope": s.scopeRefs[relation.To.Scope]}
	return scope.Match(object, metadata, predicates)
}

func (s *Service) entitiesByKeys(keys []scope.EntityKey, withSource bool) ([]Entity, error) {
	result := make([]Entity, 0, len(keys))
	for _, key := range keys {
		item, err := s.EntityByKey(key, withSource)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
func (s *Service) relationsBySet(indexes map[int]struct{}, withSource bool) []Relation {
	ordered := make([]int, 0, len(indexes))
	for index := range indexes {
		ordered = append(ordered, index)
	}
	sort.Ints(ordered)
	return s.relationsByIndexes(ordered, withSource)
}
func (s *Service) relationsByIndexes(indexes []int, withSource bool) []Relation {
	all := s.QueryRelations(nil, withSource)
	result := make([]Relation, 0, len(indexes))
	for _, index := range indexes {
		result = append(result, all[index])
	}
	return result
}
func sortedKeySet(set map[scope.EntityKey]struct{}) []scope.EntityKey {
	keys := make([]scope.EntityKey, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sortEntityKeys(keys)
	return keys
}
func reverseEntityKeys(values []scope.EntityKey) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
func reverseInts(values []int) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
