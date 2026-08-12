package git

import (
	"reflect"
	"strings"
	"testing"

	"wt/internal/gittest"
)

func TestLookupBranch(t *testing.T) {
	repo := gittest.NewRepo(t)
	gittest.AddRemote(t, repo)
	gittest.Git(t, repo, "branch", "local-only")
	gittest.Git(t, repo, "push", "origin", "main:remote-only")
	gittest.Git(t, repo, "push", "origin", "main:nested/leaf")

	cases := []struct {
		branch  string
		local   bool
		remotes []string
	}{
		{"local-only", true, nil},
		{"main", true, []string{"origin"}},
		{"remote-only", false, []string{"origin"}},
		{"nope", false, nil},
		// "leaf" only exists as nested/leaf on origin; the fnmatch * in the
		// lookup pattern must not report it as branch "leaf".
		{"leaf", false, nil},
		{"nested/leaf", false, []string{"origin"}},
	}
	for _, tc := range cases {
		refs, err := LookupBranch(repo, tc.branch)
		if err != nil {
			t.Fatalf("LookupBranch(%s): %v", tc.branch, err)
		}
		if refs.Local != tc.local || !reflect.DeepEqual(refs.Remotes, tc.remotes) {
			t.Errorf("LookupBranch(%s) = %+v, want local=%v remotes=%v", tc.branch, refs, tc.local, tc.remotes)
		}
	}
}

func TestUpstreamOf(t *testing.T) {
	repo := gittest.NewRepo(t)
	gittest.AddRemote(t, repo)

	up, ok := UpstreamOf(repo, "main")
	if !ok || up.Name != "origin/main" || up.Track != "" {
		t.Errorf("in-sync upstream = %+v ok=%v, want origin/main with empty track", up, ok)
	}

	gittest.WriteFile(t, repo, "f.txt", "x")
	gittest.Commit(t, repo, "ahead")
	up, ok = UpstreamOf(repo, "main")
	if !ok || !strings.Contains(up.Track, "ahead 1") {
		t.Errorf("diverged upstream = %+v ok=%v, want track containing 'ahead 1'", up, ok)
	}

	gittest.Git(t, repo, "branch", "floating")
	if _, ok := UpstreamOf(repo, "floating"); ok {
		t.Error("UpstreamOf(floating) ok=true, want false (no upstream configured)")
	}
}

func TestBranches(t *testing.T) {
	repo := gittest.NewRepo(t)
	gittest.AddRemote(t, repo)
	gittest.Git(t, repo, "branch", "extra")
	gittest.Git(t, repo, "push", "origin", "main:remote-only")
	gittest.Git(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	locals, remotes, err := Branches(repo)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"extra", "main"}; !reflect.DeepEqual(locals, want) {
		t.Errorf("locals = %v, want %v", locals, want)
	}
	if want := []string{"main", "remote-only"}; !reflect.DeepEqual(remotes, want) {
		t.Errorf("remotes = %v, want %v (HEAD excluded, prefix stripped)", remotes, want)
	}
}

func TestLocalBranches(t *testing.T) {
	repo := gittest.NewRepo(t)
	gittest.Git(t, repo, "branch", "main-2")
	gittest.Git(t, repo, "branch", "main-10")

	got, err := LocalBranches(repo, "main-*")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"main-10", "main-2"} // for-each-ref sorts lexically
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LocalBranches(main-*) = %v, want %v", got, want)
	}
}
