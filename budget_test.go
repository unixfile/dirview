package www

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// pageBudget is the time www promises for any page from a warm server.
const pageBudget = 5 * time.Millisecond

// Every kind of page renders within pageBudget once its git answers are
// cached: a doc of about 13,000 words, a code
// page of render.go, a listing, histories and a commit's diff. Each takes
// the median of 15 runs, so one slow run does not fail it. The doc is a
// fixed text, the README and doc/ as of 2026-11-11 with the program's
// name replaced, since WORKING_NAME alone holds it. So the measure follows
// the code and not the growth of the docs.
func TestPageBudget(t *testing.T) {
	if raceOn {
		t.Skip("the race detector slows every page past the budget")
	}
	needGit(t)
	dir := t.TempDir()
	readme, err := os.ReadFile("testdata/budget.md")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "README.md"), readme, 0o644)
	b, err := os.ReadFile("render.go")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "render.go"), b, 0o644)
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	commitAll(t, dir, "Add the README and render.go")
	s := newServer().local()
	addTree(t, s, dir, "/", "budget")
	h := s.handler()
	c := head(t, dir, "HEAD")
	for _, p := range []string{"/README.md", "/render.go", "/", "/?log", "/README.md?log", "/README.md?commit=" + c} {
		get(h, p)
		times := make([]time.Duration, 15)
		for i := range times {
			start := time.Now()
			rec := get(h, p)
			times[i] = time.Since(start)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status %d", p, rec.Code)
			}
		}
		slices.Sort(times)
		if m := times[len(times)/2]; m > pageBudget {
			t.Errorf("%s: %v, over the budget of %v", p, m, pageBudget)
		} else {
			t.Logf("%s: %v", p, m)
		}
	}
}
