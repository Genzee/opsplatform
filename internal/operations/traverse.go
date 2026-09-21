package operations

import (
	"context"
	"sort"
	"time"

	"github.com/Genzee/opsplatform/internal/repository"
	"github.com/Genzee/opsplatform/pkg/model"
)

type Traversal struct {
	ResourceID   string `json:"resource_id"`
	TargetID     string `json:"target_id,omitempty"`
	Profile      string `json:"profile"`
	Direction    string `json:"direction,omitempty"`
	IncludeStale bool   `json:"include_stale,omitempty"`
	MaxDepth     int    `json:"max_depth,omitempty"`
	MaxNodes     int    `json:"max_nodes,omitempty"`
	MaxEdges     int    `json:"max_edges,omitempty"`
}
type Frontier struct {
	ResourceID     string `json:"resource_id"`
	RelationshipID string `json:"relationship_id,omitempty"`
	Reason         string `json:"reason"`
}
type GraphResult struct {
	Root              string               `json:"root"`
	Profile           string               `json:"profile"`
	Semantics         string               `json:"semantics"`
	AsOf              time.Time            `json:"as_of"`
	Resources         []model.Resource     `json:"resources"`
	Relationships     []model.Relationship `json:"relationships"`
	Frontiers         []Frontier           `json:"frontiers"`
	IncompleteReasons []string             `json:"incomplete_reasons"`
	Truncated         bool                 `json:"truncated"`
	Found             *bool                `json:"found,omitempty"`
	Path              []string             `json:"path,omitempty"`
}

func normalize(q *Traversal) error {
	switch q.Profile {
	case "neighbors", "path", "infrastructure/v1", "dependencies/v1", "potential-impact/v1":
	default:
		return bad("unsupported traversal profile")
	}
	if q.Direction == "" {
		q.Direction = "out"
		if q.Profile == "neighbors" {
			q.Direction = "both"
		}
	}
	if q.Direction != "in" && q.Direction != "out" && q.Direction != "both" {
		return bad("direction must be in, out or both")
	}
	if q.MaxDepth == 0 {
		q.MaxDepth = 16
	}
	if q.Profile == "neighbors" {
		q.MaxDepth = 1
	}
	if q.MaxNodes == 0 {
		q.MaxNodes = 1000
	}
	if q.MaxEdges == 0 {
		q.MaxEdges = 5000
	}
	if q.MaxDepth < 1 || q.MaxDepth > 64 || q.MaxNodes < 1 || q.MaxNodes > 5000 || q.MaxEdges < 1 || q.MaxEdges > 20000 {
		return bad("limits must be depth 1..64, nodes 1..5000, edges 1..20000")
	}
	if q.Profile == "path" && q.TargetID == "" {
		return bad("target_id is required for path")
	}
	return nil
}

type step struct {
	edge    model.Relationship
	next    string
	reverse bool
}
type predecessor struct{ node, edge string }

func (s *Service) Traverse(ctx context.Context, q Traversal) (GraphResult, error) {
	if err := normalize(&q); err != nil {
		return GraphResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	snapshot, err := s.repo.Snapshot(ctx)
	if err != nil {
		return GraphResult{}, err
	}
	root, err := get(snapshot, q.ResourceID)
	if err != nil {
		return GraphResult{}, err
	}
	if q.Profile == "path" {
		if _, err := get(snapshot, q.TargetID); err != nil {
			return GraphResult{}, err
		}
	}
	result := GraphResult{Root: root.ID, Profile: q.Profile, Semantics: "observed-topology", AsOf: snapshot.AsOf,
		Resources: []model.Resource{root}, Relationships: []model.Relationship{}, Frontiers: []Frontier{}, IncompleteReasons: []string{}}
	if q.Profile == "potential-impact/v1" {
		result.Semantics = "potential-impact-not-outage"
	}
	if q.Profile == "path" {
		found := false
		result.Found = &found
	}
	byID := make(map[string]model.Resource)
	adj := make(map[string][]step)
	for _, r := range snapshot.Resources {
		byID[r.ID] = r
	}
	for _, e := range snapshot.Relationships {
		if e.Status == model.Deleted {
			continue
		}
		if e.SourceID != "" {
			adj[e.SourceID] = append(adj[e.SourceID], step{e, e.TargetID, false})
		}
		if e.TargetID != "" {
			adj[e.TargetID] = append(adj[e.TargetID], step{e, e.SourceID, true})
		}
	}
	for id := range adj {
		sort.Slice(adj[id], func(i, j int) bool {
			a, b := adj[id][i], adj[id][j]
			if a.next != b.next {
				ra, oka := byID[a.next]
				rb, okb := byID[b.next]
				if oka && okb {
					return repository.ResourceLess(ra, rb)
				}
				return a.next < b.next
			}
			if a.edge.Provider != b.edge.Provider {
				return a.edge.Provider < b.edge.Provider
			}
			return a.edge.Key < b.edge.Key
		})
	}
	reasons := map[string]bool{}
	frontiers := map[Frontier]bool{}
	mark := func(id, edge, reason string, truncated bool) {
		reasons[reason] = true
		result.Truncated = result.Truncated || truncated
		f := Frontier{id, edge, reason}
		if !frontiers[f] {
			result.Frontiers = append(result.Frontiers, f)
			frontiers[f] = true
		}
	}
	visited := map[string]int{root.ID: 0}
	edges := make(map[string]model.Relationship)
	parents := make(map[string]predecessor)
	queue := []string{root.ID}
	if root.Status == model.Stale {
		mark(root.ID, "", "stale_resource", false)
		if !q.IncludeStale {
			queue = nil
		}
	}
	if q.Profile == "path" && root.ID == q.TargetID && (root.Status != model.Stale || q.IncludeStale) {
		*result.Found = true
		result.Path = []string{root.ID}
		queue = nil
	}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		if q.Profile == "neighbors" && visited[current] == 1 {
			continue
		}
		if ctx.Err() != nil {
			mark(current, "", "query_budget_exhausted", true)
			break
		}
		for _, step := range adj[current] {
			if ctx.Err() != nil {
				mark(current, "", "query_budget_exhausted", true)
				break
			}
			e := step.edge
			if !permits(q.Profile, q.Direction, e.Source.Kind, e.Type, e.Target.Kind, step.reverse) {
				continue
			}
			if e.Pending {
				mark(current, e.ID, "unresolved_reference", false)
				continue
			}
			next := byID[step.next]
			if next.Status == model.Deleted {
				mark(current, e.ID, "deleted_endpoint", false)
				continue
			}
			stale := e.Status == model.Stale || byID[current].Status == model.Stale || next.Status == model.Stale
			if stale {
				mark(current, e.ID, "stale_evidence", false)
				if !q.IncludeStale {
					continue
				}
			}
			_, already := visited[step.next]
			if visited[current] >= q.MaxDepth {
				if !already {
					mark(current, e.ID, "max_depth", true)
				}
				continue
			}
			if _, exists := edges[e.ID]; !exists && len(edges) >= q.MaxEdges {
				mark(current, e.ID, "max_edges", true)
				continue
			}
			if !already && len(visited) >= q.MaxNodes {
				mark(current, e.ID, "max_nodes", true)
				continue
			}
			edges[e.ID] = e
			if !already {
				visited[step.next] = visited[current] + 1
				parents[step.next] = predecessor{current, e.ID}
				queue = append(queue, step.next)
				result.Resources = append(result.Resources, next)
			}
			if q.Profile == "path" && step.next == q.TargetID {
				*result.Found = true
				path := []string{q.TargetID}
				pathEdges := []model.Relationship{}
				for id := q.TargetID; id != root.ID; {
					p := parents[id]
					pathEdges = append(pathEdges, edges[p.edge])
					id = p.node
					path = append(path, id)
				}
				result.Resources = []model.Resource{}
				for i := len(path) - 1; i >= 0; i-- {
					result.Path = append(result.Path, path[i])
					result.Resources = append(result.Resources, byID[path[i]])
				}
				for i := len(pathEdges) - 1; i >= 0; i-- {
					result.Relationships = append(result.Relationships, pathEdges[i])
				}
				finish(&result, reasons)
				return result, nil
			}
		}
	}
	for _, e := range snapshot.Relationships {
		if _, ok := edges[e.ID]; ok {
			result.Relationships = append(result.Relationships, e)
		}
	}
	finish(&result, reasons)
	return result, nil
}

func finish(result *GraphResult, reasons map[string]bool) {
	for r := range reasons {
		result.IncompleteReasons = append(result.IncompleteReasons, r)
	}
	sort.Strings(result.IncompleteReasons)
	sort.Slice(result.Frontiers, func(i, j int) bool {
		a, b := result.Frontiers[i], result.Frontiers[j]
		if a.ResourceID != b.ResourceID {
			return a.ResourceID < b.ResourceID
		}
		if a.RelationshipID != b.RelationshipID {
			return a.RelationshipID < b.RelationshipID
		}
		return a.Reason < b.Reason
	})
}
