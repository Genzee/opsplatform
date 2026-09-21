package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Genzee/opsplatform/internal/fixture"
	"github.com/Genzee/opsplatform/internal/operations"
	"github.com/Genzee/opsplatform/internal/repository"
)

func registry(t *testing.T) (*Registry, *operations.Service) {
	t.Helper()
	store := repository.NewMemory()
	if err := fixture.Load(context.Background(), "../../../examples/shared-storage.json", store); err != nil {
		t.Fatal(err)
	}
	s := operations.New(store)
	return New(s), s
}

func TestToolUsesSameOperationsAndPreservesProvenance(t *testing.T) {
	r, s := registry(t)
	ctx := context.Background()
	value, err := r.Call(ctx, "find_resources", json.RawMessage(`{"kind":"nfs.Server"}`))
	if err != nil {
		t.Fatal(err)
	}
	page := value.(operations.ResourcePage)
	if len(page.Resources) != 1 {
		t.Fatal("server lookup failed")
	}
	id := page.Resources[0].ID
	args, _ := json.Marshal(map[string]string{"resource_id": id})
	value, err = r.Call(ctx, "get_impact", args)
	if err != nil {
		t.Fatal(err)
	}
	toolResult := value.(operations.GraphResult)
	direct, err := s.Traverse(ctx, operations.Traversal{ResourceID: id, Profile: "potential-impact/v1"})
	if err != nil {
		t.Fatal(err)
	}
	toolResult.AsOf = direct.AsOf
	if !reflect.DeepEqual(toolResult, direct) {
		t.Fatal("tool semantics differ from service semantics")
	}
	if len(toolResult.Relationships) == 0 || toolResult.Relationships[0].Provenance.Source == "" {
		t.Fatal("tool lost evidence")
	}
}

func TestUntrustedArgumentsCannotSelectUnregisteredOperations(t *testing.T) {
	r, _ := registry(t)
	cases := []struct{ name, args, code string }{
		{"execute_shell", `{"command":"anything"}`, "UNSUPPORTED_CAPABILITY"},
		{"create_plan", `{}`, "UNSUPPORTED_CAPABILITY"},
		{"get_impact", `{}`, "INVALID_ARGUMENT"},
		{"get_impact", `{"resource_id":"x","profile":"path"}`, "INVALID_ARGUMENT"},
		{"get_impact", `{"resource_id":null}`, "INVALID_ARGUMENT"},
		{"get_impact", `{"resource_id":"x","max_nodes":0}`, "INVALID_ARGUMENT"},
		{"get_impact", `{"resource_id":"x","max_depth":1.2}`, "INVALID_ARGUMENT"},
		{"get_impact", `{"resource_id":"x","include_stale":"true"}`, "INVALID_ARGUMENT"},
		{"get_impact", `{"resource_id":"x","include_stale":null}`, "INVALID_ARGUMENT"},
		{"get_neighbors", `{"resource_id":"x","direction":"everywhere"}`, "INVALID_ARGUMENT"},
		{"find_resources", `{"limit":1001}`, "INVALID_ARGUMENT"},
		{"find_resources", `null`, "INVALID_ARGUMENT"},
		{"get_capabilities", `{} {}`, "INVALID_ARGUMENT"},
	}
	for _, tc := range cases {
		t.Run(tc.name+tc.args, func(t *testing.T) {
			_, err := r.Call(context.Background(), tc.name, json.RawMessage(tc.args))
			var typed *operations.Error
			if !errors.As(err, &typed) || typed.Code != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
		})
	}
}

func TestStdioSurvivesBadRequestsAndAdvertisesOnlyAvailableTools(t *testing.T) {
	r, _ := registry(t)
	in := bytes.NewBufferString("not json\n" + `{"id":"one","method":"list_tools"}` + "\n" + `{"id":"two","method":"call_tool","name":"get_capabilities","arguments":{}}` + "\n")
	var out bytes.Buffer
	if err := r.Serve(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(&out)
	var first, second, third Response
	if err := dec.Decode(&first); err != nil {
		t.Fatal(err)
	}
	if err := dec.Decode(&second); err != nil {
		t.Fatal(err)
	}
	if err := dec.Decode(&third); err != nil {
		t.Fatal(err)
	}
	if first.Error == nil || second.Error != nil || third.Error != nil || string(second.ID) != `"one"` {
		t.Fatal("bad stream response")
	}
	defs := second.Result.([]any)
	if len(defs) != 8 {
		t.Fatalf("unexpected operation catalog: %d", len(defs))
	}
	for _, d := range defs {
		if d.(map[string]any)["name"] == "execute_plan" {
			t.Fatal("advertised unimplemented operation")
		}
	}
}
