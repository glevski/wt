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

	seen := newInodeSet()
	size, depsSize, shared, err := diskUsage(root, []string{"node_modules"}, seen)
	if err != nil {
		t.Fatal(err)
	}
	if size <= 0 || depsSize <= 0 || shared <= 0 {
		t.Fatalf("size=%d depsSize=%d shared=%d, want all > 0", size, depsSize, shared)
	}
	if depsSize >= size {
		t.Errorf("depsSize %d should be a strict subset of size %d", depsSize, size)
	}
	if seen.total >= size {
		t.Errorf("unique total %d should be < naive size %d (the hardlinked twin dedupes)", seen.total, size)
	}

	// a second root sharing the same inode adds nothing to the unique total
	root2 := t.TempDir()
	if err := os.Link(filepath.Join(root, "twin.js"), filepath.Join(root2, "twin.js")); err != nil {
		t.Fatal(err)
	}
	before := seen.total
	if _, _, _, err := diskUsage(root2, nil, seen); err != nil {
		t.Fatal(err)
	}
	if seen.total != before {
		t.Errorf("unique total grew by %d for an already-seen inode", seen.total-before)
	}
}

func TestDf(t *testing.T) {
	repo, _ := depsFixture(t)
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
