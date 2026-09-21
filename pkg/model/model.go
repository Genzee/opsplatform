// Package model defines concrete resources and provider-owned relationship facts.
// Kinds and relationship types are open strings, not closed provider enums.
package model

import "time"

type Ref struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	NativeID string `json:"native_id"`
}

type Lifecycle string

const (
	Active  Lifecycle = "ACTIVE"
	Stale   Lifecycle = "STALE"
	Deleted Lifecycle = "DELETED"
)

type Observation struct {
	Ref        Ref            `json:"ref"`
	Name       string         `json:"name"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Status     Lifecycle      `json:"status,omitempty"`
}

type Resource struct {
	ID string `json:"id"`
	Observation
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

type Provenance struct {
	Method         string   `json:"method"`
	Source         string   `json:"source"`
	SourceRevision string   `json:"source_revision,omitempty"`
	Evidence       []string `json:"evidence,omitempty"`
	Rule           string   `json:"rule,omitempty"`
}

// Fact is one provider's assertion, not a merged logical edge. Its key is
// immutable within that provider; changing endpoints requires a new key.
type Fact struct {
	Provider   string     `json:"provider"`
	Key        string     `json:"key"`
	Source     Ref        `json:"source"`
	Target     Ref        `json:"target"`
	Type       string     `json:"type"`
	Provenance Provenance `json:"provenance"`
	Status     Lifecycle  `json:"status,omitempty"`
}

type Relationship struct {
	ID string `json:"id"`
	Fact
	SourceID    string    `json:"source_id,omitempty"`
	TargetID    string    `json:"target_id,omitempty"`
	Pending     bool      `json:"pending"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"`
}

// Batch is a local, atomic fixture/import operation. It is NOT the future
// authenticated, ordered provider protocol (epochs and coverage are not here).
type Batch struct {
	Resources     []Observation `json:"resources"`
	Relationships []Fact        `json:"relationships"`
}

type Snapshot struct {
	AsOf          time.Time      `json:"as_of"`
	Resources     []Resource     `json:"resources"`
	Relationships []Relationship `json:"relationships"`
}
