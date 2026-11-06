package peers

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/hev/kit/internal/dispatch"
)

// RPCRequest travels inside dispatch.Request.Note with action "peers". SSH
// executes only the fixed dispatch RPC command, never peers arguments as shell.
type RPCRequest struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin"`
}

func HandleRPC(ctx context.Context, note string) dispatch.Response {
	var q RPCRequest
	if err := json.Unmarshal([]byte(note), &q); err != nil {
		return dispatch.Response{Error: "invalid peers request"}
	}
	var out, errout bytes.Buffer
	local := New()
	local.remote = true
	err := local.Run(ctx, q.Args, strings.NewReader(q.Stdin), &out, &errout)
	result := dispatch.Response{Log: out.String(), Message: errout.String()}
	if err != nil {
		result.Error = err.Error()
	}
	return result
}
