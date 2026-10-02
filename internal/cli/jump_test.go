package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEmitJumpJSON(t *testing.T) {
	t.Setenv("WT_JUMP", "json")
	out, _ := setupOutputs(t)

	emitJump("/tmp/it's <here> & there", "/tmp/home")

	if strings.Count(out.String(), "\n") != 1 {
		t.Errorf("stdout = %q, want exactly one line", out.String())
	}
	var jump jumpJSON
	if err := json.Unmarshal(out.Bytes(), &jump); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if jump.Cd != "/tmp/it's <here> & there" || jump.Home != "/tmp/home" {
		t.Errorf("jump = %+v", jump)
	}
}

func TestEmitJumpJSONWithoutHome(t *testing.T) {
	t.Setenv("WT_JUMP", "json")
	out, _ := setupOutputs(t)

	emitJump("/tmp/dest", "")

	if got, want := out.String(), `{"cd":"/tmp/dest"}`+"\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// A jump command end to end: same resolution and side effects, JSON on stdout.
func TestCheckoutJSONJump(t *testing.T) {
	repo, _, fixPath := checkoutFixture(t)
	t.Setenv("WT_JUMP", "json")
	out, _ := setupOutputs(t)

	if err := checkout(repo, []string{"feature-f"}); err != nil {
		t.Fatal(err)
	}
	var jump jumpJSON
	if err := json.Unmarshal(out.Bytes(), &jump); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if jump.Cd != fixPath || jump.Home != repo {
		t.Errorf("jump = %+v, want cd %s home %s", jump, fixPath, repo)
	}
}
