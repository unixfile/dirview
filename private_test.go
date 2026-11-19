package dirview

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// privateWeb is a web that reads private repos from f with token, or
// fails to with err when token is "".
func privateWeb(t *testing.T, f *forge, token string, err error) *web {
	t.Helper()
	w := testWeb(t, defaultDisk)
	oldRoot, oldAuth, oldHost := privateRoot, privateAuth, privateHost
	t.Cleanup(func() { privateRoot, privateAuth, privateHost = oldRoot, oldAuth, oldHost })
	if err := w.usePrivate(filepath.Join(t.TempDir(), progName)); err != nil {
		t.Fatal(err)
	}
	privateHost = f.host
	privateAuth = func() (string, error) { return token, err }
	depths[f.host] = 3
	t.Cleanup(func() { delete(depths, f.host) })
	f.private = "acct"
	f.auth = "Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:sesame"))
	return w
}

// A repo that asks for a login is read with gh's login into the private
// cache, and its pages show it; the public cache keeps nothing of it.
func TestPrivateRepoReadsThroughGh(t *testing.T) {
	f := newForge(t, nil)
	f.repo(t, "acct/secret", map[string]string{"README.md": "# Hidden\n"})
	w := privateWeb(t, f, "sesame", nil)
	rec := read(t, w, "/"+f.host+"/acct/secret/README.md")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Hidden") {
		t.Fatalf("%d\n%s", rec.Code, rec.Body)
	}
	if !exists(filepath.Join(w.private, f.host, "acct", "secret", ".git", "HEAD")) {
		t.Error("the repo is not in the private cache")
	}
	if exists(filepath.Join(w.root, f.host)) {
		t.Error("the public cache keeps a directory of the private repo")
	}
	// A fetch sends the login too.
	w.mu.Lock()
	r := w.repos[f.host+"/acct/secret"]
	w.mu.Unlock()
	f.repo(t, "acct/secret", map[string]string{"README.md": "# Moved\n"})
	w.refresh(r)
	if r.failed != "" {
		t.Errorf("fetch failed: %s", r.failed)
	}
}

// Without gh's login the page says what is missing.
func TestPrivateRepoWithoutGh(t *testing.T) {
	f := newForge(t, nil)
	f.repo(t, "acct/secret", map[string]string{"README.md": "# Hidden\n"})
	w := privateWeb(t, f, "", &errno{syscall.ENOENT, progName + " reads private repos through gh, which is not installed"})
	rec := read(t, w, "/"+f.host+"/acct/secret/README.md")
	if body := rec.Body.String(); !strings.Contains(body, "gh, which is not installed") || strings.Contains(body, "Hidden") {
		t.Fatalf("%d\n%s", rec.Code, body)
	}
	if entries, _ := os.ReadDir(w.private); len(entries) > 0 {
		t.Errorf("the private cache holds %v", entries)
	}
}

// A public repo on the same forge is read without the login, into the
// public cache.
func TestPublicRepoSkipsGh(t *testing.T) {
	f := newForge(t, nil)
	f.repo(t, "open/docs", map[string]string{"README.md": "# Open\n"})
	w := privateWeb(t, f, "", errors.New("gh must not be asked"))
	if rec := read(t, w, "/"+f.host+"/open/docs/README.md"); rec.Code != http.StatusOK {
		t.Fatalf("%d\n%s", rec.Code, rec.Body)
	}
	if !exists(filepath.Join(w.root, f.host, "open", "docs", ".git", "HEAD")) {
		t.Error("the public repo is not in the public cache")
	}
}

// The login reaches git through the environment, never its arguments,
// and only for the private cache's host.
func TestPrivateEnv(t *testing.T) {
	oldAuth, oldHost := privateAuth, privateHost
	t.Cleanup(func() { privateAuth, privateHost = oldAuth, oldHost })
	privateAuth = func() (string, error) { return "sesame", nil }
	privateHost = "github.com"
	env := strings.Join(privateEnv(), "\n")
	if !strings.Contains(env, "GIT_CONFIG_KEY_0=http."+webScheme+"://github.com/.extraHeader") {
		t.Errorf("env %q", env)
	}
	privateAuth = nil
	if env := privateEnv(); env != nil {
		t.Errorf("without a login: %q", env)
	}
}

// The web address answers the account that runs it, and refuses another
// account or one it cannot tell.
func TestOwnerOnly(t *testing.T) {
	ok := ownerOnly(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	srv := httptest.NewUnstartedServer(ok)
	srv.Config.ConnContext = peerUID
	srv.Start()
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("own account: %d", resp.StatusCode)
	}
	for _, uid := range []any{os.Getuid() + 1, -1, nil} {
		req := httptest.NewRequest("GET", "/", nil)
		if uid != nil {
			req = req.WithContext(withPeer(req, uid.(int)))
		}
		rec := httptest.NewRecorder()
		ok.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("uid %v: %d", uid, rec.Code)
		}
	}
}

func withPeer(r *http.Request, uid int) context.Context {
	return context.WithValue(r.Context(), peerKey{}, uid)
}
