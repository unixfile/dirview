package dirview

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// commitAll commits every change in dir with message msg.
func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@x", "commit", "-qm", msg}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// repoTree is serveTree in a git repo with two commits: one adds every
// file, the next changes a/b.go. untracked/ is in no commit.
func repoTree(t *testing.T) (*server, string) {
	t.Helper()
	needGit(t)
	s, _, dir := serveTree(t)
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.lock\n"), 0o644)
	commitAll(t, dir, "Add the docs\n\nThe first line\nwraps here.\n- a list item\n- another\n\nA second paragraph.")
	os.WriteFile(filepath.Join(dir, "a", "b.go"), []byte("package b\n"), 0o644)
	commitAll(t, dir, "Rename the package <b>")
	os.Mkdir(filepath.Join(dir, "untracked"), 0o755)
	os.WriteFile(filepath.Join(dir, "untracked", "x.txt"), []byte("x\n"), 0o644)
	return s, dir
}

// Loopback keeps trailers.
func TestLoopbackKeepsTrailers(t *testing.T) {
	s, dir := repoTree(t)
	commitAll(t, dir, "Third\n\nSigned-off-by: t <t@x>")
	if body := get(s.handler(), "/?log").Body.String(); !strings.Contains(body, "Signed-off-by: t &lt;t@x&gt;") {
		t.Errorf("trailer gone on loopback:\n%s", body)
	}
}

func TestDropTrailers(t *testing.T) {
	for in, want := range map[string]string{
		"Body.\n\nKey: v\nOther-Key: w":                                    "Body.",
		"Claude-Session: https://x":                                        "",
		"Body.\n\nNot a trailer line":                                      "Body.\n\nNot a trailer line",
		"Body.\n\nKey: v\nplain":                                           "Body.",
		"Body.\n\n(cherry picked from commit abc)\nSigned-off-by: t <t@x>": "Body.",
		"Body.\n\nCo-authored-by: a\n  <a@x>\nKey: v":                      "Body.",
		"Body.\n\nOne\ntwo\nthree\nfour\nNote: five":                       "Body.\n\nOne\ntwo\nthree\nfour\nNote: five",
		"": "",
	} {
		if got := dropTrailers(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// head is the hash of HEAD in dir.
func head(t *testing.T, dir, rev string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", rev).Output()
	if err != nil {
		t.Fatalf("rev-parse %s: %v", rev, err)
	}
	return strings.TrimSpace(string(out))
}

// A new tree's listing and README have their top lines cached before the
// first visit.
func TestWarmCachesFirstPages(t *testing.T) {
	_, dir := repoTree(t)
	os.WriteFile(filepath.Join(dir, "ReadMe.md"), []byte("# Hi\n"), 0o644)
	commitAll(t, dir, "Add a readme")
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	tr := newTree(root, dir, "/", "w")
	tr.warm()
	r := repoAt(t.Context(), dir)
	for _, name := range []string{".", "ReadMe.md"} {
		d, spec, _ := tr.gitPlace(name)
		key := fmt.Sprintf("log\x00%s\x00%s\x00%s\x00%s\x00%d\x00%t", r.gitDir, r.head(t.Context()), d, spec, 2, false)
		if _, ok := gitCache.get(key); !ok {
			t.Errorf("%s: top line not cached", name)
		}
	}
}

// A repo's fsmonitor program never runs, even for a command that reads
// the index.
func TestGitRunsNoFsmonitor(t *testing.T) {
	_, dir := repoTree(t)
	ran := filepath.Join(t.TempDir(), "ran")
	mon := filepath.Join(dir, ".git", "mon.sh")
	os.WriteFile(mon, []byte("#!/bin/sh\ntouch "+ran+"\n"), 0o755)
	if out, err := exec.Command("git", "-C", dir, "config", "core.fsmonitor", mon).CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	git(t.Context(), dir, "status")
	if _, err := os.Stat(ran); err == nil {
		t.Error("git ran the repo's fsmonitor")
	}
}

// When a git call's context ends, the helpers git started die with it,
// and the call returns without waiting for them: on 2026-11-19 a fetch's
// git-remote-https outlived its bound by 13 hours.
func TestGitCancelKillsHelpers(t *testing.T) {
	_, dir := repoTree(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	git(ctx, dir, "-c", "alias.hang=!echo $$ >"+pidFile+"; exec sleep 30", "hang")
	if d := time.Since(start); d >= waitPipes {
		t.Errorf("git returned after %v, want before the helper closed its pipes", d)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("helper never started: %v", err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	for i := 0; ; i++ {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			break
		}
		if i == 40 {
			t.Fatalf("helper %d still runs after the call returned", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// git in the cache starts no automatic gc or maintenance, and the git
// processes it starts, eg a lazy fetch, inherit that.
func TestCacheGitRunsNoAutoGc(t *testing.T) {
	old := cacheRoot
	cacheRoot = t.TempDir()
	t.Cleanup(func() { cacheRoot = old })
	dir := filepath.Join(cacheRoot, "repo")
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	out, err := git(t.Context(), dir, "-c", "alias.child=!git config --get gc.auto; git config --get maintenance.auto", "child")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "0\nfalse\n" {
		t.Errorf("child git sees gc.auto and maintenance.auto as %q, want \"0\\nfalse\\n\"", got)
	}
}
