package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// OpenSpecState asks `openspec instructions apply --change <change> --json`
// (run in root) for the change's apply state: "ready", "blocked" or
// "all_done", with the CLI's instruction as the message.
func OpenSpecState(ctx context.Context, root, change string) (state, message string, err error) {
	cmd := exec.CommandContext(ctx, "openspec", "instructions", "apply", "--change", change, "--json")
	cmd.Dir = root
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", "", fmt.Errorf("openspec: %s", msg)
	}
	return parseApplyState(out.Bytes())
}

func parseApplyState(b []byte) (string, string, error) {
	var resp struct {
		State       string `json:"state"`
		Instruction string `json:"instruction"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return "", "", fmt.Errorf("openspec: unexpected output: %v", err)
	}
	if resp.State == "" {
		return "", "", errors.New("openspec: no state in the apply instructions")
	}
	return resp.State, resp.Instruction, nil
}
