package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"wt/internal/git"
)

// depsWorker is the hidden `__deps-worker` entry point. It runs detached from
// the shell, so its only observable outputs are the deps-state marker and the
// log file in the destination's git admin dir.
func depsWorker(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: worktree __deps-worker <src> <dst> <dep>...")
	}
	src, dst, deps := args[0], args[1], args[2:]
	// Record our pid so a forced `wt rm` can stop us instead of racing the
	// copy; the final done/failed write below supersedes it.
	git.WriteDepsState(dst, fmt.Sprintf("copying %d", os.Getpid()))
	if err := copyDepsInto(src, dst, deps); err != nil {
		git.WriteDepsState(dst, "failed: "+err.Error())
		return err
	}
	git.WriteDepsState(dst, "done")
	return nil
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

// copyDepsInto copies deps one at a time; each lands atomically — built in a
// .wt-partial sibling and renamed into place — so a half-copied node_modules
// is never visible at its real path.
func copyDepsInto(src, dst string, deps []string) error {
	for _, dep := range deps {
		from := filepath.Join(src, dep)
		to := filepath.Join(dst, dep)
		tmp := to + ".wt-partial"
		_ = os.RemoveAll(tmp)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		info, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = copyTree(from, tmp)
		} else {
			err = copyPath(from, tmp)
		}
		if err != nil {
			return fmt.Errorf("copying %s: %w", dep, err)
		}
		_ = os.RemoveAll(to)
		if err := os.Rename(tmp, to); err != nil {
			return fmt.Errorf("placing %s: %w", dep, err)
		}
	}
	return nil
}

// startDepsWorker relaunches the binary as a detached background worker. A
// package var so tests can stub it — the test binary must never re-exec
// itself. Stdio goes to the admin-dir log file: inheriting our stdout would
// keep the wt() wrapper's $(...) capture open and hang the shell.
var startDepsWorker = func(src, dst string, deps []string) error {
	bin, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(git.DepsLogPath(dst), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(bin, append([]string{"__deps-worker", src, dst}, deps...)...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
