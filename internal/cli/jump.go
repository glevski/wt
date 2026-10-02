package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// jumpJSON is a jump for callers that can't eval shell code.
type jumpJSON struct {
	Cd   string `json:"cd"`
	Home string `json:"home,omitempty"`
}

// emitJump prints the script the wt() wrapper evals on a jump: cd to the
// destination plus the WT_HOME convenience variable pointing at the repo's
// main checkout. An empty home skips the export (destination outside any
// known repo), keeping whatever WT_HOME was set before.
//
// With WT_JUMP=json in the environment the same jump is one JSON line
// instead — for editor integrations, where a jump means opening a folder.
func emitJump(dest, home string) {
	if os.Getenv("WT_JUMP") == "json" {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(jumpJSON{Cd: dest, Home: home})
		return
	}
	fmt.Fprintf(stdout, "cd %s\n", shellQuote(dest))
	if home != "" {
		fmt.Fprintf(stdout, "export WT_HOME=%s\n", shellQuote(home))
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
