package git

import "strings"

// BranchRefs says where a branch exists.
type BranchRefs struct {
	Local   bool
	Remotes []string // remotes that have the branch
}

// LookupBranch checks local heads and every remote in a single git call.
func LookupBranch(dir, branch string) (BranchRefs, error) {
	out, err := Run(dir, "for-each-ref", "--format=%(refname)",
		"refs/heads/"+branch, "refs/remotes/*/"+branch)
	if err != nil {
		return BranchRefs{}, err
	}
	var refs BranchRefs
	for _, ref := range strings.Split(out, "\n") {
		if ref == "refs/heads/"+branch {
			refs.Local = true
			continue
		}
		// The * in the pattern is fnmatch and can swallow slashes, matching
		// e.g. refs/remotes/origin/nested/<branch>; keep only refs where the
		// part after the remote name is exactly the branch.
		rest := strings.TrimPrefix(ref, "refs/remotes/")
		if rest == ref {
			continue
		}
		remote, name, ok := strings.Cut(rest, "/")
		if ok && name == branch {
			refs.Remotes = append(refs.Remotes, remote)
		}
	}
	return refs, nil
}

// Upstream describes a branch's configured upstream.
type Upstream struct {
	Name  string // e.g. "origin/main"
	Track string // "ahead 1, behind 2", "gone", or "" when in sync
}

// UpstreamOf returns branch's upstream, or ok=false when none is configured.
func UpstreamOf(dir, branch string) (up Upstream, ok bool) {
	out, err := Run(dir, "for-each-ref",
		"--format=%(upstream:short)\t%(upstream:track,nobracket)", "refs/heads/"+branch)
	if err != nil {
		return Upstream{}, false
	}
	name, track, _ := strings.Cut(out, "\t")
	if name == "" {
		return Upstream{}, false
	}
	return Upstream{Name: name, Track: track}, true
}

// LocalBranches lists local branches matching pattern (fnmatch, e.g. "main-*").
func LocalBranches(dir, pattern string) ([]string, error) {
	out, err := Run(dir, "for-each-ref", "--format=%(refname:short)", "refs/heads/"+pattern)
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}
