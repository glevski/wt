package git

import (
	"reflect"
	"testing"

	"wt/internal/gittest"
)

func TestChangeLists(t *testing.T) {
	repo := gittest.NewRepo(t)
	gittest.WriteFile(t, repo, "staged.txt", "base")
	gittest.WriteFile(t, repo, "unstaged.txt", "base")
	gittest.Commit(t, repo, "baseline")

	gittest.WriteFile(t, repo, "staged.txt", "changed")
	gittest.Git(t, repo, "add", "staged.txt")
	gittest.WriteFile(t, repo, "unstaged.txt", "changed")
	gittest.WriteFile(t, repo, "new/untracked.txt", "new")

	staged, err := StagedFiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	unstaged, err := UnstagedFiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	untracked, err := UntrackedFiles(repo)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(staged, []string{"staged.txt"}) {
		t.Errorf("staged = %v", staged)
	}
	if !reflect.DeepEqual(unstaged, []string{"unstaged.txt"}) {
		t.Errorf("unstaged = %v", unstaged)
	}
	if !reflect.DeepEqual(untracked, []string{"new/untracked.txt"}) {
		t.Errorf("untracked = %v", untracked)
	}
}

func TestChangeListsClean(t *testing.T) {
	repo := gittest.NewRepo(t)
	for name, fn := range map[string]func(string) ([]string, error){
		"staged": StagedFiles, "unstaged": UnstagedFiles, "untracked": UntrackedFiles,
	} {
		got, err := fn(repo)
		if err != nil || len(got) != 0 {
			t.Errorf("%s on clean repo = %v, %v; want empty, nil", name, got, err)
		}
	}
}
