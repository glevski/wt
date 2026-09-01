package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"wt/internal/git"
)

// depsWorker is the hidden `__deps-worker [--link]` entry point. It runs
// detached from the shell, so its only observable outputs are the deps-state
// marker and the log file in the destination's git admin dir.
func depsWorker(args []string) error {
	link := false
	if len(args) > 0 && args[0] == "--link" {
		link = true
		args = args[1:]
	}
	if len(args) < 3 {
		return fmt.Errorf("usage: worktree __deps-worker [--link] <src> <dst> <dep>...")
	}
	src, dst, deps := args[0], args[1], args[2:]
	// Record our pid so a forced `wt rm` can stop us instead of racing the
	// copy; the final linked/done/failed write below supersedes it.
	git.WriteDepsState(dst, fmt.Sprintf("copying %d", os.Getpid()))
	linked, err := transferDepsInto(src, dst, deps, link)
	if err != nil {
		git.WriteDepsState(dst, "failed: "+err.Error())
		return err
	}
	git.WriteDepsState(dst, doneState(linked))
	return nil
}

// doneState names the terminal deps state: "linked" while any dep shares
// hardlinked inodes with its source, plain "done" for owned copies.
func doneState(linked bool) string {
	if linked {
		return "linked"
	}
	return "done"
}

// stopDepsWorker kills a still-running deps worker (its pid rides in the
// "copying <pid>" marker) so a forced removal doesn't race the copy. Best
// effort: no pid recorded yet, or a long-dead process, are both fine.
func stopDepsWorker(worktreePath string) {
	state, ok := git.DepsState(worktreePath)
	if !ok || !strings.HasPrefix(state, "copying") {
		return
	}
	var pid int
	if _, err := fmt.Sscanf(state, "copying %d", &pid); err != nil || pid <= 0 {
		return
	}
	// Negative pid targets the worker's process group (it setsid'd itself).
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}

// transferDepsInto brings deps over one at a time; each lands atomically —
// built in a .wt-partial sibling and renamed into place — so a half-copied
// node_modules is never visible at its real path. With link, regular files
// become hardlinks to the source (near-zero space); a dep on another
// filesystem falls back to a plain copy, since hardlinks cannot cross
// devices. Reports whether any dep ended up hardlinked.
func transferDepsInto(src, dst string, deps []string, link bool) (linked bool, err error) {
	for _, dep := range deps {
		from := filepath.Join(src, dep)
		to := filepath.Join(dst, dep)
		tmp := to + ".wt-partial"
		_ = os.RemoveAll(tmp)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return linked, err
		}
		info, err := os.Lstat(from)
		if err != nil {
			return linked, err
		}
		stage := func(useLink bool) error {
			switch {
			case info.IsDir() && useLink:
				return linkTree(from, tmp)
			case info.IsDir():
				return copyTree(from, tmp)
			case useLink:
				return linkPath(from, tmp)
			default:
				return copyPath(from, tmp)
			}
		}
		depLinked := link
		err = stage(link)
		if err != nil && link && errors.Is(err, syscall.EXDEV) {
			logf("'%s' sits on another filesystem — copying instead of linking", dep)
			_ = os.RemoveAll(tmp)
			depLinked = false
			err = stage(false)
		}
		if err != nil {
			return linked, fmt.Errorf("bringing over %s: %w", dep, err)
		}
		linked = linked || depLinked
		_ = os.RemoveAll(to)
		if err := os.Rename(tmp, to); err != nil {
			return linked, fmt.Errorf("placing %s: %w", dep, err)
		}
	}
	return linked, nil
}

// startDepsWorker relaunches the binary as a detached background worker. A
// package var so tests can stub it — the test binary must never re-exec
// itself. Stdio goes to the admin-dir log file: inheriting our stdout would
// keep the wt() wrapper's $(...) capture open and hang the shell.
var startDepsWorker = func(src, dst string, deps []string, link bool) error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(git.DepsLogPath(dst), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	workerArgs := []string{"__deps-worker"}
	if link {
		workerArgs = append(workerArgs, "--link")
	}
	cmd := exec.Command(bin, append(append(workerArgs, src, dst), deps...)...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
