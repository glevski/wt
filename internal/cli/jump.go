package cli

import (
	"fmt"
	"strings"
)

// emitJump prints the script the wt() wrapper evals on a jump: cd to the
// destination plus the WT_HOME convenience variable pointing at the repo's
// main checkout. An empty home skips the export (destination outside any
// known repo), keeping whatever WT_HOME was set before.
func emitJump(dest, home string) {
	fmt.Fprintf(stdout, "cd %s\n", shellQuote(dest))
	if home != "" {
		fmt.Fprintf(stdout, "export WT_HOME=%s\n", shellQuote(home))
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
