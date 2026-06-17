package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// ResolveRequest is the stdin payload from the Consi/OpenClaw exec provider protocol.
type ResolveRequest struct {
	ProtocolVersion int      `json:"protocolVersion"`
	Provider        string   `json:"provider"`
	IDs             []string `json:"ids"`
	// Timestamp is accepted but ignored (kc has no TTL/rotation).
	Timestamp string `json:"timestamp,omitempty"`
}

// ResolveResponse is the stdout payload.
type ResolveResponse struct {
	ProtocolVersion int                `json:"protocolVersion"`
	Values          map[string]*string `json:"values"`
}

func newResolveCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve multiple secret IDs from stdin JSON and print values to stdout",
		Long: `Implements the Consi/OpenClaw exec provider protocol.

Reads a JSON request from stdin:
  {"protocolVersion":1,"provider":"kc","ids":["KEY1","KEY2"]}

Writes a JSON response to stdout:
  {"protocolVersion":1,"values":{"KEY1":"val1","KEY2":null}}

Unknown keys are returned as null. Protected keys trigger a single Touch ID
prompt before resolution.

Examples:
  echo '{"protocolVersion":1,"provider":"kc","ids":["OPENAI_API_KEY"]}' | kc resolve
  kc resolve --vault prod < request.json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResolve(cmd, app)
		},
	}
	return cmd
}

func runResolve(cmd *cobra.Command, app *App) error {
	// Read all of stdin.
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("resolve: read stdin: %w", err)
	}
	if len(in) == 0 {
		return fmt.Errorf("resolve: no input on stdin")
	}

	var req ResolveRequest
	if err := json.Unmarshal(in, &req); err != nil {
		return fmt.Errorf("resolve: invalid JSON: %w", err)
	}

	if len(req.IDs) == 0 {
		// Empty request — return empty values.
		resp := ResolveResponse{
			ProtocolVersion: 1,
			Values:          map[string]*string{},
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(resp)
	}

	vault, err := app.resolveVault(cmd)
	if err != nil {
		return err
	}

	// Check protection: if any requested key is protected, trigger Touch ID once.
	metadata, err := app.Store.ListMetadata(vault)
	if err != nil {
		return fmt.Errorf("resolve: list metadata: %w", err)
	}
	if anyProtected(metadata, req.IDs) {
		session := authSession(app)
		if err := session.Authorize("Unlock kc secrets"); err != nil {
			return err
		}
	}

	// Resolve each ID.
	values := make(map[string]*string, len(req.IDs))
	for _, id := range req.IDs {
		if id == "" {
			values[id] = nil
			continue
		}
		value, err := app.Store.Get(vault, id)
		if err != nil {
			// Unknown key → null, not an error.
			values[id] = nil
			continue
		}
		v := value
		values[id] = &v
	}

	resp := ResolveResponse{
		ProtocolVersion: 1,
		Values:          values,
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("resolve: write stdout: %w", err)
	}
	return nil
}

// anyProtected returns true if any of the given IDs is marked as protected
// in the vault metadata.
func anyProtected(metadata []SecretMetadata, ids []string) bool {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	for _, item := range metadata {
		if idSet[item.Key] && item.Protection == ProtectionProtected {
			return true
		}
	}
	return false
}
