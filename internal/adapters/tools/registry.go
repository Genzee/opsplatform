// Package tools exposes vendor-neutral function tools. A model SDK can register
// Definitions and route each function call to Call. No MCP/OpenAPI dependency.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Genzee/opsplatform/internal/operations"
)

type Definition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}
type Registry struct{ service *operations.Service }

func New(service *operations.Service) *Registry { return &Registry{service: service} }

func text(description string) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "description": description}
}
func integer(min, max int) map[string]any {
	return map[string]any{"type": "integer", "minimum": min, "maximum": max}
}
func object(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}
func traversalSchema(path, direction bool) map[string]any {
	p := map[string]any{
		"resource_id":   text("Exact resource ID returned by find_resources; do not invent IDs."),
		"include_stale": map[string]any{"type": "boolean"},
		"max_depth":     integer(1, 64), "max_nodes": integer(1, 5000), "max_edges": integer(1, 20000),
	}
	required := []string{"resource_id"}
	if path {
		p["target_id"] = text("Exact target resource ID.")
		required = append(required, "target_id")
	}
	if direction {
		p["direction"] = map[string]any{"type": "string", "enum": []string{"in", "out", "both"}}
	}
	return object(p, required...)
}

func (r *Registry) Definitions() []Definition {
	return []Definition{
		{"find_resources", "Find observed resources by exact filters. Names may be ambiguous: use returned IDs. Data is local fixture data in this M1 runtime.", object(map[string]any{
			"kind": text("Concrete resource kind, e.g. kubernetes.Pod."), "name": text("Exact display name."),
			"provider": text("Logical provider ID."), "namespace": text("Kubernetes namespace."),
			"include_stale": map[string]any{"type": "boolean"}, "limit": integer(1, 1000), "offset": integer(0, 20000),
		})},
		{"get_resource", "Read a resource, its native identity, lifecycle and attributes by ID.", object(map[string]any{"resource_id": text("Exact resource ID.")}, "resource_id")},
		{"get_neighbors", "Inspect adjacent facts with provenance. Adjacency alone does not imply failure or dependency.", traversalSchema(false, true)},
		{"get_infrastructure", "Trace supported runtime and OS inventory relations; report stale or unresolved frontiers.", traversalSchema(false, false)},
		{"get_dependencies", "Follow supported workload and backing relations. This does not prove traffic or health.", traversalSchema(false, false)},
		{"get_impact", "Return potential impact through supported dependency and ownership rules. This is not an outage determination or root-cause diagnosis.", traversalSchema(false, false)},
		{"find_path", "Find one shortest-hop path; directed out by default. A path is not proof of causality.", traversalSchema(true, true)},
		{"get_capabilities", "List implemented operations and runtime limitations. Diagnosis, provisioning and execution are not implemented.", object(map[string]any{})},
	}
}

func decode(data json.RawMessage, target any) error {
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return &operations.Error{Code: "INVALID_ARGUMENT", Message: "arguments must be a JSON object"}
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return &operations.Error{Code: "INVALID_ARGUMENT", Message: err.Error()}
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return &operations.Error{Code: "INVALID_ARGUMENT", Message: "arguments must contain one JSON object"}
	}
	return nil
}

// validateArguments mirrors the small schema subset emitted above. Validation
// must happen locally even if a remote model provider also validates tool input.
func validateArguments(schema map[string]any, data json.RawMessage) error {
	var values map[string]json.RawMessage
	if err := decode(data, &values); err != nil {
		return err
	}
	for _, key := range schema["required"].([]string) {
		if _, ok := values[key]; !ok {
			return &operations.Error{Code: "INVALID_ARGUMENT", Message: key + " is required"}
		}
	}
	props := schema["properties"].(map[string]any)
	for key, raw := range values {
		p, ok := props[key]
		if !ok {
			return &operations.Error{Code: "INVALID_ARGUMENT", Message: "unknown argument: " + key}
		}
		rule := p.(map[string]any)
		invalid := bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
		switch rule["type"] {
		case "string":
			var value string
			if err := json.Unmarshal(raw, &value); err != nil || value == "" {
				invalid = true
			}
			if choices, ok := rule["enum"].([]string); ok {
				found := false
				for _, choice := range choices {
					found = found || value == choice
				}
				invalid = invalid || !found
			}
		case "integer":
			var value int
			if err := json.Unmarshal(raw, &value); err != nil || value < rule["minimum"].(int) || value > rule["maximum"].(int) {
				invalid = true
			}
		case "boolean":
			var value bool
			if err := json.Unmarshal(raw, &value); err != nil {
				invalid = true
			}
		}
		if invalid {
			return &operations.Error{Code: "INVALID_ARGUMENT", Message: "invalid value for " + key}
		}
	}
	return nil
}

func (r *Registry) Call(ctx context.Context, name string, data json.RawMessage) (any, error) {
	if len(data) > 1<<20 {
		return nil, &operations.Error{Code: "INVALID_ARGUMENT", Message: "arguments exceed 1 MiB"}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var definition *Definition
	for _, d := range r.Definitions() {
		if d.Name == name {
			copy := d
			definition = &copy
			break
		}
	}
	if definition == nil {
		return nil, &operations.Error{Code: "UNSUPPORTED_CAPABILITY", Message: fmt.Sprintf("tool %q is not implemented", name)}
	}
	if err := validateArguments(definition.InputSchema, data); err != nil {
		return nil, err
	}
	switch name {
	case "get_capabilities":
		return map[string]any{"runtime": "local-fixture-memory", "operations": r.Definitions(), "limitations": []string{
			"synthetic observations; no live discovery", "no automatic identity resolution or coverage tracking",
			"no diagnosis, provisioning or mutation tools", "no network listener or authentication", "state is lost on process exit",
			"registered built-in traversal profiles only; no external profile loader yet",
		}}, nil
	case "find_resources":
		var f operations.Filter
		if err := decode(data, &f); err != nil {
			return nil, err
		}
		return r.service.FindResources(ctx, f)
	case "get_resource":
		var arg struct {
			ID string `json:"resource_id"`
		}
		if err := decode(data, &arg); err != nil {
			return nil, err
		}
		return r.service.GetResource(ctx, arg.ID)
	default:
		var q operations.Traversal
		if err := decode(data, &q); err != nil {
			return nil, err
		}
		profiles := map[string]string{"get_neighbors": "neighbors", "get_infrastructure": "infrastructure/v1", "get_dependencies": "dependencies/v1", "get_impact": "potential-impact/v1", "find_path": "path"}
		q.Profile = profiles[name]
		return r.service.Traverse(ctx, q)
	}
}
