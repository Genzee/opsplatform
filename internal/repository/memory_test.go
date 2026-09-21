package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Genzee/opsplatform/pkg/model"
)

func TestIdentityAtomicityAndPendingReferences(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	m := NewMemoryWithClock(func() time.Time { return now })
	a := model.Observation{Ref: model.Ref{Provider: "test", Kind: "custom.A", NativeID: "uid-a"}, Name: "same-name", Attributes: map[string]any{"nested": map[string]any{"value": "original"}}}
	b := model.Observation{Ref: model.Ref{Provider: "test", Kind: "custom.A", NativeID: "uid-b"}, Name: "same-name"}
	fact := model.Fact{Provider: "test", Key: "edge", Source: a.Ref, Target: b.Ref, Type: "CUSTOM", Provenance: model.Provenance{Method: "observed", Source: "fixture"}}
	if err := m.Apply(ctx, model.Batch{Resources: []model.Observation{a}, Relationships: []model.Fact{fact}}); err != nil {
		t.Fatal(err)
	}
	s, _ := m.Snapshot(ctx)
	id, first := s.Resources[0].ID, s.Resources[0].FirstSeenAt
	if !s.Relationships[0].Pending {
		t.Fatal("missing endpoint should be pending")
	}
	a.Attributes["nested"].(map[string]any)["value"] = "caller mutation"
	s.Resources[0].Attributes["nested"].(map[string]any)["value"] = "snapshot mutation"
	s, _ = m.Snapshot(ctx)
	if s.Resources[0].Attributes["nested"].(map[string]any)["value"] != "original" {
		t.Fatal("repository leaked mutable state")
	}
	now = now.Add(time.Minute)
	if err := m.Apply(ctx, model.Batch{Resources: []model.Observation{a, b}, Relationships: []model.Fact{fact}}); err != nil {
		t.Fatal(err)
	}
	s, _ = m.Snapshot(ctx)
	if len(s.Resources) != 2 || s.Resources[0].ID != id || s.Resources[1].ID == id || !s.Resources[0].FirstSeenAt.Equal(first) || !s.Resources[0].LastSeenAt.Equal(now) {
		t.Fatal("identity or timestamp contract violated")
	}
	if s.Relationships[0].Pending {
		t.Fatal("late endpoint did not resolve")
	}
	beforeID := s.Relationships[0].ID
	if err := m.Apply(ctx, model.Batch{Relationships: []model.Fact{fact}}); err != nil {
		t.Fatal(err)
	}
	s, _ = m.Snapshot(ctx)
	if len(s.Relationships) != 1 || s.Relationships[0].ID != beforeID {
		t.Fatal("duplicate fact created another assertion")
	}
	bad := fact
	bad.Target = a.Ref
	a.Name = "must-not-commit"
	if err := m.Apply(ctx, model.Batch{Resources: []model.Observation{a}, Relationships: []model.Fact{bad}}); err == nil {
		t.Fatal("assertion identity change accepted")
	}
	s, _ = m.Snapshot(ctx)
	if s.Resources[0].Name == a.Name {
		t.Fatal("failed batch partially committed")
	}
}

func TestProvenanceAndConcurrentAccess(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	a := model.Observation{Ref: model.Ref{Provider: "p", Kind: "kind", NativeID: "one"}, Name: "one"}
	f := model.Fact{Provider: "p", Key: "edge", Source: a.Ref, Target: a.Ref, Type: "loop"}
	if err := m.Apply(ctx, model.Batch{Resources: []model.Observation{a}, Relationships: []model.Fact{f}}); err == nil {
		t.Fatal("accepted unexplained edge")
	}
	s, _ := m.Snapshot(ctx)
	if len(s.Resources) != 0 {
		t.Fatal("invalid batch committed resource")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if err := m.Apply(ctx, model.Batch{Resources: []model.Observation{a}}); err != nil {
					t.Error(err)
				}
				if _, err := m.Snapshot(ctx); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}
