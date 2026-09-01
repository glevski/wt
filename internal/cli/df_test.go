package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/gittest"
)

func TestDiskUsage(t *testing.T) {
	root := t.TempDir()
	gittest.WriteFile(t, root, "app.js", strings.Repeat("a", 5000))
	gittest.WriteFile(t, root, "node_modules/dep/big.js", strings.Repeat("b", 8000))
	if err := os.Link(filepath.Join(root, "node_modules/dep/big.js"), filepath.Join(root, "twin.js")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("app.js", filepath.Join(root, "alias.js")); err != nil {
		t.Fatal(err)
	}

	u, err := diskUsage(root, []string{"node_modules"})
	if err != nil {
		t.Fatal(err)
	}
	if u.own <= 0 || u.ownDeps != 0 {
		t.Errorf("own=%d ownDeps=%d; want own > 0 (app.js) and no single-link dep bytes", u.own, u.ownDeps)
	}
	if len(u.shared) != 1 {
		t.Fatalf("shared inodes = %d, want 1 (the in-tree twin counts once, like du)", len(u.shared))
	}
	for key, bytes := range u.shared {
		if bytes < 8000 {
			t.Errorf("shared bytes = %d, want at least the file's blocks", bytes)
		}
		if _, ok := u.sharedDeps[key]; !ok {
			t.Error("the shared inode lives under node_modules — sharedDeps must contain it")
		}
	}
}

func TestDf(t *testing.T) {
	repo, root := depsFixture(t)
	setupOutputs(t)
	if err := create(repo, []string{"-w"}); err != nil {
		t.Fatal(err)
	}
	out, _ := setupOutputs(t)

	if err := df(repo, nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"NAME", "DEPS-SIZE", "SHARED", "main-2", "linked", "TOTAL", "saved by sharing"} {
		if !strings.Contains(got, want) {
			t.Errorf("df output missing %q:\n%s", want, got)
		}
	}
	// first-owner attribution: the root holds the shared inodes (canonical
	// order starts there), so the linked fork's SIZE must be tiny — smaller
	// than its own SHARED subset.
	for _, line := range strings.Split(got, "\n") {
		if !strings.Contains(line, "main-2") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			t.Fatalf("unexpected row: %q", line)
		}
		size, shared := fields[2], fields[4]
		if size == shared {
			t.Errorf("linked fork's SIZE (%s) should exclude shared bytes (%s) — first owner pays", size, shared)
		}
	}
	_ = root
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:                  "0B",
		512:                "512B",
		2048:               "2.0K",
		10 * 1024:          "10K",
		5 * 1024 * 1024:    "5.0M",
		3 << 30:            "3.0G",
		1536 * 1024 * 1024: "1.5G",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
