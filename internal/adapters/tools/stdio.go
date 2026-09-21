package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/Genzee/opsplatform/internal/operations"
)

type Request struct {
	ID        json.RawMessage `json:"id,omitempty"`
	Method    string          `json:"method"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}
type Response struct {
	ID     json.RawMessage   `json:"id,omitempty"`
	Result any               `json:"result,omitempty"`
	Error  *operations.Error `json:"error,omitempty"`
}

// Serve is a local JSON-lines development adapter, not MCP or JSON-RPC.
// It is sequential, emits only response JSON on stdout, and opens no sockets.
func (r *Registry) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	encoder := json.NewEncoder(out)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var req Request
		err := decode(json.RawMessage(scanner.Bytes()), &req)
		response := Response{ID: req.ID}
		if err == nil {
			switch req.Method {
			case "list_tools":
				response.Result = r.Definitions()
			case "call_tool":
				response.Result, err = r.Call(ctx, req.Name, req.Arguments)
			default:
				err = &operations.Error{Code: "INVALID_ARGUMENT", Message: "method must be list_tools or call_tool"}
			}
		}
		if err != nil {
			response.Result = nil
			var typed *operations.Error
			if errors.As(err, &typed) {
				response.Error = typed
			} else {
				response.Error = &operations.Error{Code: "INTERNAL", Message: "operation failed"}
			}
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read request (maximum line 1 MiB): %w", err)
	}
	return nil
}
