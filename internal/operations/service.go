// Package operations is the shared application service used by CLI and tools.
// It has no model-vendor, HTTP, Kubernetes client or Linux collector dependency.
package operations

import (
	"context"
	"fmt"
	"time"

	"github.com/Genzee/opsplatform/internal/repository"
	"github.com/Genzee/opsplatform/pkg/model"
)

type Error struct {
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Candidates []string `json:"candidates,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func bad(message string) error { return &Error{Code: "INVALID_ARGUMENT", Message: message} }

type Service struct{ repo repository.Reader }

func New(repo repository.Reader) *Service { return &Service{repo: repo} }

type Filter struct {
	Kind         string `json:"kind,omitempty"`
	Name         string `json:"name,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	IncludeStale bool   `json:"include_stale,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Offset       int    `json:"offset,omitempty"`
}
type ResourcePage struct {
	Resources  []model.Resource `json:"resources"`
	AsOf       time.Time        `json:"as_of"`
	NextOffset *int             `json:"next_offset,omitempty"`
}

func matches(r model.Resource, f Filter) bool {
	return (f.Kind == "" || r.Ref.Kind == f.Kind) && (f.Name == "" || r.Name == f.Name) &&
		(f.Provider == "" || r.Ref.Provider == f.Provider) &&
		(f.Namespace == "" || r.Attributes["k8s.namespace.name"] == f.Namespace)
}

func (s *Service) FindResources(ctx context.Context, f Filter) (ResourcePage, error) {
	if f.Limit == 0 {
		f.Limit = 100
	}
	if f.Limit < 1 || f.Limit > 1000 || f.Offset < 0 {
		return ResourcePage{}, bad("limit must be 1..1000 and offset nonnegative")
	}
	snapshot, err := s.repo.Snapshot(ctx)
	if err != nil {
		return ResourcePage{}, err
	}
	page := ResourcePage{Resources: []model.Resource{}, AsOf: snapshot.AsOf}
	seen := 0
	for _, r := range snapshot.Resources {
		if err := ctx.Err(); err != nil {
			return ResourcePage{}, err
		}
		if r.Status == model.Deleted || (!f.IncludeStale && r.Status == model.Stale) || !matches(r, f) {
			continue
		}
		seen++
		if seen <= f.Offset {
			continue
		}
		if len(page.Resources) == f.Limit {
			next := f.Offset + len(page.Resources)
			page.NextOffset = &next
			break
		}
		page.Resources = append(page.Resources, r)
	}
	return page, nil
}

func (s *Service) GetResource(ctx context.Context, id string) (model.Resource, error) {
	snapshot, err := s.repo.Snapshot(ctx)
	if err != nil {
		return model.Resource{}, err
	}
	return get(snapshot, id)
}
func get(snapshot model.Snapshot, id string) (model.Resource, error) {
	if id == "" {
		return model.Resource{}, bad("resource_id is required")
	}
	for _, r := range snapshot.Resources {
		if r.ID != id {
			continue
		}
		if r.Status == model.Deleted {
			return model.Resource{}, &Error{Code: "RESOURCE_DELETED", Message: "resource is a retained tombstone"}
		}
		return r, nil
	}
	return model.Resource{}, &Error{Code: "RESOURCE_NOT_FOUND", Message: fmt.Sprintf("resource %q was not found", id)}
}

// Resolve is convenience for human callers. Tools use IDs from FindResources.
func (s *Service) Resolve(ctx context.Context, name string, f Filter) (string, error) {
	snapshot, err := s.repo.Snapshot(ctx)
	if err != nil {
		return "", err
	}
	for _, r := range snapshot.Resources {
		if r.ID == name {
			_, err := get(snapshot, name)
			return name, err
		}
	}
	f.Name = name
	candidates := []string{}
	for _, r := range snapshot.Resources {
		if r.Status != model.Deleted && matches(r, f) {
			candidates = append(candidates, r.ID)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) == 0 {
		return "", &Error{Code: "RESOURCE_NOT_FOUND", Message: "no resource matches the selector"}
	}
	return "", &Error{Code: "AMBIGUOUS_RESOURCE", Message: "use a resource ID or a narrower kind/provider/namespace filter", Candidates: candidates}
}
