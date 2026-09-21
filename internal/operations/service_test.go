package operations_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/Genzee/opsplatform/internal/fixture"
	"github.com/Genzee/opsplatform/internal/operations"
	"github.com/Genzee/opsplatform/internal/repository"
	"github.com/Genzee/opsplatform/pkg/model"
)

func setup(t *testing.T) (*repository.Memory, *operations.Service) {
	t.Helper()
	m := repository.NewMemory()
	if err := fixture.Load(context.Background(), "../../examples/shared-storage.json", m); err != nil {
		t.Fatal(err)
	}
	return m, operations.New(m)
}
func resolve(t *testing.T, s *operations.Service, name string) string {
	t.Helper()
	id, err := s.Resolve(context.Background(), name, operations.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSharedStorageImpactIsNotNamespaceBlastRadius(t *testing.T) {
	_, s := setup(t)
	ctx := context.Background()
	r, err := s.Traverse(ctx, operations.Traversal{ResourceID: resolve(t, s, "storage-01"), Profile: "potential-impact/v1"})
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, resource := range r.Resources {
		if resource.Ref.Kind == "kubernetes.Deployment" {
			names = append(names, resource.Name)
		}
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{"payment-api", "report-api"}) {
		t.Fatalf("wrong impact: %v", names)
	}
	if r.Semantics != "potential-impact-not-outage" || r.Truncated {
		t.Fatalf("wrong semantics: %+v", r)
	}
	for _, edge := range r.Relationships {
		if edge.Provenance.Source == "" {
			t.Fatal("lost provenance")
		}
	}
	r, err = s.Traverse(ctx, operations.Traversal{ResourceID: resolve(t, s, "worker-1-os"), Profile: "potential-impact/v1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range r.Resources {
		if resource.Name == "report-api" {
			t.Fatal("node impact crossed shared inventory")
		}
	}
	page, _ := s.FindResources(ctx, operations.Filter{Kind: "linux.NetworkInterface", Provider: "linux/worker-1"})
	r, err = s.Traverse(ctx, operations.Traversal{ResourceID: page.Resources[0].ID, Profile: "potential-impact/v1"})
	if err != nil || len(r.Resources) != 1 {
		t.Fatal("NIC membership promoted to whole-OS failure")
	}
}

func TestBoundsFreshnessDeletionAndUnknownKinds(t *testing.T) {
	m, s := setup(t)
	ctx := context.Background()
	root := resolve(t, s, "payment-api")
	for _, limit := range []operations.Traversal{
		{ResourceID: root, Profile: "infrastructure/v1", MaxNodes: 1},
		{ResourceID: root, Profile: "infrastructure/v1", MaxEdges: 1},
		{ResourceID: root, Profile: "infrastructure/v1", MaxDepth: 1},
	} {
		r, err := s.Traverse(ctx, limit)
		if err != nil || !r.Truncated || len(r.IncompleteReasons) == 0 {
			t.Fatalf("missing truncation: %+v %v", r, err)
		}
	}
	page, _ := s.FindResources(ctx, operations.Filter{Kind: "kubernetes.Node", Name: "worker-1"})
	node := page.Resources[0].Observation
	node.Status = model.Stale
	if err := m.Apply(ctx, model.Batch{Resources: []model.Observation{node}}); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Traverse(ctx, operations.Traversal{ResourceID: root, Profile: "infrastructure/v1"})
	for _, resource := range r.Resources {
		if resource.Ref == node.Ref {
			t.Fatal("stale resource traversed by default")
		}
	}
	if len(r.IncompleteReasons) == 0 {
		t.Fatal("stale frontier silently omitted")
	}
	r, _ = s.Traverse(ctx, operations.Traversal{ResourceID: root, Profile: "infrastructure/v1", IncludeStale: true})
	found := false
	for _, resource := range r.Resources {
		found = found || resource.Ref == node.Ref
	}
	if !found {
		t.Fatal("include_stale ignored")
	}
	node.Status = model.Deleted
	if err := m.Apply(ctx, model.Batch{Resources: []model.Observation{node}}); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Traverse(ctx, operations.Traversal{ResourceID: root, Profile: "infrastructure/v1", IncludeStale: true})
	for _, resource := range r.Resources {
		if resource.Ref == node.Ref {
			t.Fatal("deleted resource traversed")
		}
	}
	a := model.Observation{Ref: model.Ref{Provider: "third-party", Kind: "future.Host", NativeID: "1"}, Name: "future-host"}
	b := model.Observation{Ref: model.Ref{Provider: "third-party", Kind: "future.Device", NativeID: "2"}, Name: "future-device"}
	f := model.Fact{Provider: "third-party", Key: "future", Source: a.Ref, Target: b.Ref, Type: "BACKED_BY", Provenance: model.Provenance{Method: "observed", Source: "fixture"}}
	if err := m.Apply(ctx, model.Batch{Resources: []model.Observation{a, b}, Relationships: []model.Fact{f}}); err != nil {
		t.Fatal(err)
	}
	id := resolve(t, s, "future-device")
	r, _ = s.Traverse(ctx, operations.Traversal{ResourceID: id, Profile: "potential-impact/v1"})
	if len(r.Resources) != 1 {
		t.Fatal("unknown kind gained implicit impact semantics")
	}
	r, _ = s.Traverse(ctx, operations.Traversal{ResourceID: id, Profile: "neighbors"})
	if len(r.Resources) != 2 {
		t.Fatal("unknown relationship lost to generic neighbor query")
	}
}

func TestShortestPathCyclesAmbiguityAndPending(t *testing.T) {
	m := repository.NewMemory()
	s := operations.New(m)
	ctx := context.Background()
	var observations []model.Observation
	for _, n := range []string{"a", "b", "c", "d"} {
		observations = append(observations, model.Observation{Ref: model.Ref{Provider: "p", Kind: "custom.Kind", NativeID: n}, Name: n})
	}
	facts := []model.Fact{}
	for i, pair := range [][2]int{{0, 2}, {2, 3}, {0, 1}, {1, 3}, {3, 0}} {
		facts = append(facts, model.Fact{Provider: "p", Key: string(rune('a' + i)), Source: observations[pair[0]].Ref, Target: observations[pair[1]].Ref, Type: "LINK", Provenance: model.Provenance{Method: "observed", Source: "fixture"}})
	}
	if err := m.Apply(ctx, model.Batch{Resources: observations, Relationships: facts}); err != nil {
		t.Fatal(err)
	}
	a, d := resolve(t, s, "a"), resolve(t, s, "d")
	for i := 0; i < 3; i++ {
		r, err := s.Traverse(ctx, operations.Traversal{ResourceID: a, TargetID: d, Profile: "path"})
		if err != nil || !*r.Found || len(r.Path) != 3 || r.Resources[1].Name != "b" {
			t.Fatalf("unstable shortest path: %+v %v", r, err)
		}
	}
	r, err := s.Traverse(ctx, operations.Traversal{ResourceID: a, TargetID: a, Profile: "path"})
	if err != nil || !*r.Found || len(r.Path) != 1 {
		t.Fatal("zero hop path failed")
	}
	pending := facts[0]
	pending.Key = "pending"
	pending.Target.NativeID = "not-yet-seen"
	if err := m.Apply(ctx, model.Batch{Relationships: []model.Fact{pending}}); err != nil {
		t.Fatal(err)
	}
	r, _ = s.Traverse(ctx, operations.Traversal{ResourceID: a, Profile: "neighbors"})
	if !reflect.DeepEqual(r.IncompleteReasons, []string{"unresolved_reference"}) {
		t.Fatalf("pending frontier missing: %+v", r)
	}
	copy := observations[0]
	copy.Ref.NativeID = "new-a"
	if err := m.Apply(ctx, model.Batch{Resources: []model.Observation{copy}}); err != nil {
		t.Fatal(err)
	}
	_, err = s.Resolve(ctx, "a", operations.Filter{})
	var typed *operations.Error
	if !errors.As(err, &typed) || typed.Code != "AMBIGUOUS_RESOURCE" || len(typed.Candidates) != 2 {
		t.Fatal("name ambiguity not detected")
	}
	page, err := s.FindResources(ctx, operations.Filter{Limit: 2})
	if err != nil || page.NextOffset == nil || *page.NextOffset != 2 {
		t.Fatal("pagination missing")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.Traverse(ctx, operations.Traversal{ResourceID: a, Profile: "neighbors"}); err == nil {
		t.Fatal("ignored cancellation")
	}
}
