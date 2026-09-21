package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Genzee/opsplatform/pkg/model"
)

type Reader interface {
	Snapshot(context.Context) (model.Snapshot, error)
}

type assertionKey struct{ provider, key string }

// Memory keeps only current state. Snapshot copying is deliberately limited to
// the small M1 fixture runtime; a persistent repository needs indexed read views.
type Memory struct {
	mu            sync.RWMutex
	resources     map[model.Ref]model.Resource
	relationships map[assertionKey]model.Relationship
	clock         func() time.Time
}

func NewMemory() *Memory { return NewMemoryWithClock(time.Now) }

func NewMemoryWithClock(clock func() time.Time) *Memory {
	return &Memory{resources: make(map[model.Ref]model.Resource), relationships: make(map[assertionKey]model.Relationship), clock: clock}
}

func validRef(ref model.Ref) bool {
	return strings.TrimSpace(ref.Provider) != "" && strings.TrimSpace(ref.Kind) != "" && strings.TrimSpace(ref.NativeID) != ""
}

func validState(state model.Lifecycle) bool {
	return state == "" || state == model.Active || state == model.Stale || state == model.Deleted
}

func validate(batch model.Batch) error {
	refs := make(map[model.Ref]bool)
	for _, r := range batch.Resources {
		if !validRef(r.Ref) || strings.TrimSpace(r.Name) == "" || !validState(r.Status) {
			return fmt.Errorf("resource requires a native ref, name and valid lifecycle")
		}
		if refs[r.Ref] {
			return fmt.Errorf("duplicate resource ref in batch: %v", r.Ref)
		}
		refs[r.Ref] = true
	}
	keys := make(map[assertionKey]bool)
	for _, f := range batch.Relationships {
		if strings.TrimSpace(f.Provider) == "" || strings.TrimSpace(f.Key) == "" || strings.TrimSpace(f.Type) == "" || !validRef(f.Source) || !validRef(f.Target) || !validState(f.Status) {
			return fmt.Errorf("relationship requires provider, key, type, refs and valid lifecycle")
		}
		if strings.TrimSpace(f.Provenance.Source) == "" {
			return fmt.Errorf("relationship %q requires provenance source", f.Key)
		}
		switch f.Provenance.Method {
		case "observed", "deterministic", "explicit_binding":
		default:
			return fmt.Errorf("relationship %q has unsupported provenance method", f.Key)
		}
		if f.Provenance.Method == "deterministic" && (f.Provenance.Rule == "" || len(f.Provenance.Evidence) == 0) {
			return fmt.Errorf("deterministic relationship %q requires rule and evidence", f.Key)
		}
		key := assertionKey{f.Provider, f.Key}
		if keys[key] {
			return fmt.Errorf("duplicate assertion key in batch: %s", f.Key)
		}
		keys[key] = true
	}
	return nil
}

func newID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b[:])
}

func (m *Memory) Apply(ctx context.Context, batch model.Batch) error {
	// Clone before taking ownership. Also reject non-JSON attributes.
	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("invalid attributes: %w", err)
	}
	if len(data) > 4<<20 {
		return fmt.Errorf("batch exceeds 4 MiB")
	}
	var owned model.Batch
	if err := json.Unmarshal(data, &owned); err != nil {
		return err
	}
	if err := validate(owned); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	newResources, newFacts := 0, 0
	for _, r := range owned.Resources {
		if _, ok := m.resources[r.Ref]; !ok {
			newResources++
		}
	}
	for _, f := range owned.Relationships {
		old, exists := m.relationships[assertionKey{f.Provider, f.Key}]
		if exists && (old.Source != f.Source || old.Target != f.Target || old.Type != f.Type) {
			return fmt.Errorf("assertion %q cannot change source, target or type", f.Key)
		}
		if !exists {
			newFacts++
		}
	}
	if len(m.resources)+newResources > 20000 || len(m.relationships)+newFacts > 50000 {
		return fmt.Errorf("M1 in-memory capacity exceeded")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	now := m.clock().UTC()
	for _, obs := range owned.Resources {
		if obs.Status == "" {
			obs.Status = model.Active
		}
		r, exists := m.resources[obs.Ref]
		if !exists {
			r.ID, r.FirstSeenAt = newID("r_"), now
		}
		r.Observation, r.LastSeenAt = obs, now
		m.resources[obs.Ref] = r
	}
	for _, fact := range owned.Relationships {
		if fact.Status == "" {
			fact.Status = model.Active
		}
		key := assertionKey{fact.Provider, fact.Key}
		r, exists := m.relationships[key]
		if !exists {
			r.ID, r.FirstSeenAt = newID("e_"), now
		}
		r.Fact, r.LastSeenAt = fact, now
		m.relationships[key] = r
	}
	return nil
}

func (m *Memory) Snapshot(ctx context.Context) (model.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return model.Snapshot{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := model.Snapshot{AsOf: m.clock().UTC(), Resources: []model.Resource{}, Relationships: []model.Relationship{}}
	for _, r := range m.resources {
		s.Resources = append(s.Resources, r)
	}
	for _, e := range m.relationships {
		if r, ok := m.resources[e.Source]; ok {
			e.SourceID = r.ID
		}
		if r, ok := m.resources[e.Target]; ok {
			e.TargetID = r.ID
		}
		e.Pending = e.SourceID == "" || e.TargetID == ""
		s.Relationships = append(s.Relationships, e)
	}
	sort.Slice(s.Resources, func(i, j int) bool { return ResourceLess(s.Resources[i], s.Resources[j]) })
	sort.Slice(s.Relationships, func(i, j int) bool {
		a, b := s.Relationships[i], s.Relationships[j]
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.Key < b.Key
	})
	// Callers cannot mutate repository state through maps or slices.
	data, err := json.Marshal(s)
	if err != nil {
		return model.Snapshot{}, err
	}
	var clone model.Snapshot
	err = json.Unmarshal(data, &clone)
	return clone, err
}

func ResourceLess(a, b model.Resource) bool {
	if a.Ref.Kind != b.Ref.Kind {
		return a.Ref.Kind < b.Ref.Kind
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	if a.Ref.Provider != b.Ref.Provider {
		return a.Ref.Provider < b.Ref.Provider
	}
	return a.Ref.NativeID < b.Ref.NativeID
}
