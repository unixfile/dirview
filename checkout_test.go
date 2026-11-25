package dirview

import (
	"bufio"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoKey reads every form of remote URL git takes for a repo on a host.
func TestRepoKey(t *testing.T) {
	b, err := os.ReadFile("testdata/repokey.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		remote, want, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatalf("no tab: %q", line)
		}
		if want == "-" {
			want = ""
		}
		if got := repoKey(remote); got != want {
			t.Errorf("repoKey(%q) = %q, want %q", remote, got, want)
		}
	}
}

// configValue takes the last value in the section, as git does, and
// compares the section's name without case and the subsection's with it.
func TestConfigValue(t *testing.T) {
	for _, c := range []struct{ config, want string }{
		{"[remote \"origin\"]\n\turl = https://a/b\n", "https://a/b"},
		{"[Remote \"origin\"]\n\tURL=\"https://a/b\"\n", "https://a/b"},
		{"[remote \"Origin\"]\n\turl = https://a/b\n", ""},
		{"[remote \"upstream\"]\n\turl = https://a/up\n[remote \"origin\"]\n\tfetch = +refs/heads/*\n", ""},
		{"[remote \"origin\"]\n\turl = https://a/one\n# url = https://a/no\n\turl = https://a/two\n[core]\n\turl = https://a/core\n", "https://a/two"},
		{"[remote]\n\turl = https://a/b\n", ""},
	} {
		if got := configValue(bufio.NewScanner(strings.NewReader(c.config)), `remote "origin"`, "url"); got != c.want {
			t.Errorf("%q: %q, want %q", c.config, got, c.want)
		}
	}
}

// gitDo runs git in dir and fails the test on an error.
func gitDo(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A cached repo's page links the same page in the user's checkout, found
// by its origin in a tree the default address serves: the file where the
// checkout has it, else the checkout's root.
func TestWebLinksTheCheckout(t *testing.T) {
	f := newForge(t, nil)
	f.repo(t, "acct/docs", map[string]string{"README.md": "# Docs\n", "guide/a.md": "Guide\n", "old.md": "Old\n"})
	f.repo(t, "acct/other", map[string]string{"README.md": "# Other\n"})
	w := testWeb(t, defaultDisk)

	// The served tree holds the checkout two levels down, as
	// ~/scratch holds github.com/owner/repo, and a repo of another
	// origin beside it.
	home := newServer().local()
	scratch := t.TempDir()
	co := filepath.Join(scratch, "host", "docs")
	os.MkdirAll(co, 0o755)
	gitDo(t, co, "init", "-q")
	gitDo(t, co, "remote", "add", "origin", "http://"+f.host+"/acct/docs.git")
	os.MkdirAll(filepath.Join(co, "guide"), 0o755)
	os.WriteFile(filepath.Join(co, "guide", "a.md"), []byte("Guide, edited\n"), 0o644)
	os.WriteFile(filepath.Join(co, "new.md"), []byte("Not pushed\n"), 0o644)
	stranger := filepath.Join(scratch, "host", "stranger")
	os.MkdirAll(stranger, 0o755)
	gitDo(t, stranger, "init", "-q")
	gitDo(t, stranger, "remote", "add", "origin", "https://example.org/x/stranger")
	addTree(t, home, scratch, rooted("/scratch/"), "~/scratch")
	w.checkouts = &checkouts{home: home, origin: "http://127.31.1.1"}
	t.Cleanup(w.checkouts.wg.Wait)
	// settled reads path once the scan its first page set going is done.
	settled := func(path string) *httptest.ResponseRecorder {
		read(t, w, path)
		w.checkouts.wg.Wait()
		return webGet(w, path)
	}

	base := "/" + f.host + "/acct/docs/"
	for _, c := range []struct{ path, want string }{
		{"guide/a.md", "/scratch/host/docs/guide/a.md"},
		{"guide/", "/scratch/host/docs/guide/"},
		{"", "/scratch/host/docs/"},
		// The file is gone from the checkout: the link leads to its root.
		{"old.md", "/scratch/host/docs/"},
		// The file is not pushed: the cached repo lacks it, the checkout
		// has it.
		{"new.md", "/scratch/host/docs/new.md"},
	} {
		rec := settled(base + c.path)
		want := `<a href="http://127.31.1.1` + c.want + `" title="this page in your checkout, ` + tildePath(co) + `">checkout</a>`
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d, no %s in\n%s", c.path, rec.Code, want, rec.Body)
		}
	}
	// A repo with no checkout has no link.
	if body := settled("/" + f.host + "/acct/other/").Body.String(); strings.Contains(body, ">checkout</a>") {
		t.Errorf("a repo without a checkout links one:\n%s", body)
	}
	// A tree that serves the checkout itself wins over the tree around
	// it, so the link names the inner tree's URL, which lookup gives the
	// same page.
	inner := addTree(t, home, co, rooted("/docs/"), "~/docs")
	if body := settled(base + "guide/a.md").Body.String(); !strings.Contains(body, `href="http://127.31.1.1/docs/guide/a.md"`) {
		t.Errorf("the inner tree loses:\n%s", body)
	}
	// A tree that stops being served takes its checkout's link with it
	// at once, before a scan has run.
	home.remove(inner)
	home.remove(home.lookup(rooted("/scratch/")))
	if body := webGet(w, base+"guide/a.md").Body.String(); strings.Contains(body, ">checkout</a>") {
		t.Errorf("a checkout no tree serves is linked:\n%s", body)
	}
}

// A checkout deeper than checkoutDepth below a tree's root, or inside a
// dot directory, goes unseen, so a scan of a large tree stays short; a
// worktree's .git file leads to the repo's config.
func TestScanCheckouts(t *testing.T) {
	needGit(t)
	root := t.TempDir()
	mk := func(rel, origin string) string {
		d := filepath.Join(root, filepath.FromSlash(rel))
		os.MkdirAll(d, 0o755)
		gitDo(t, d, "init", "-q")
		gitDo(t, d, "remote", "add", "origin", origin)
		return d
	}
	mk("a/b/c", "https://github.com/x/three")
	mk("a/b/c/d", "https://github.com/x/inner")
	mk("a/b/c2/d", "https://github.com/x/four")
	mk(".hidden/r", "https://github.com/x/hidden")
	main := mk("m", "https://github.com/x/main")
	os.WriteFile(filepath.Join(main, "f"), []byte("x\n"), 0o644)
	commitAll(t, main, "one")
	gitDo(t, main, "worktree", "add", "-q", filepath.Join(root, "wt"))
	// Two checkouts of one repo: the nearer the root wins.
	mk("z/dup", "https://github.com/x/dup")
	mk("dup", "git@github.com:x/dup.git")

	s := newServer().local()
	tr := addTree(t, s, root, rooted("/"), "root")
	got := map[string]string{}
	for k, co := range scanCheckouts([]*tree{tr}) {
		got[k] = co.rel
	}
	want := map[string]string{"github.com/x/three": "a/b/c", "github.com/x/main": "m", "github.com/x/dup": "dup"}
	// A worktree is a checkout of the same repo, as near the root as m,
	// so the first by path wins.
	for k, rel := range want {
		if got[k] != rel {
			t.Errorf("%s: %q, want %q", k, got[k], rel)
		}
	}
	for _, k := range []string{"github.com/x/inner", "github.com/x/four", "github.com/x/hidden"} {
		if rel, ok := got[k]; ok {
			t.Errorf("%s found at %q", k, rel)
		}
	}
	if co := filepath.Join(root, "wt"); originURL(co) != "https://github.com/x/main" {
		t.Errorf("the worktree's origin: %q", originURL(co))
	}
}
