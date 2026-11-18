package www

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// useWeasyprint makes weasyprint report prog as installed, or missing
// when prog is "".
func useWeasyprint(t *testing.T, prog string) {
	t.Helper()
	old := weasyprint
	weasyprint = func() (string, error) {
		if prog == "" {
			return "", errors.New("not found")
		}
		return prog, nil
	}
	t.Cleanup(func() { weasyprint = old })
}

// A Markdown page links its PDF beside raw only while weasyprint is
// installed. Without it, ?pdf says what is missing. A code page has no
// PDF.
func TestPDFLinkFollowsWeasyprint(t *testing.T) {
	s, _, _ := serveTree(t)
	h := s.handler()
	link := `<a href="?pdf" title="save this page as PDF, drawn by weasyprint">pdf</a>`

	useWeasyprint(t, "/usr/bin/weasyprint")
	if body := get(h, "/index.md").Body.String(); !strings.Contains(body, `<a href="?raw">raw</a>`+link) {
		t.Errorf("no pdf link beside raw:\n%s", body)
	}
	if body := get(h, "/a/b.go").Body.String(); strings.Contains(body, link) {
		t.Errorf("a code page links a PDF:\n%s", body)
	}

	useWeasyprint(t, "")
	if body := get(h, "/index.md").Body.String(); strings.Contains(body, link) {
		t.Errorf("pdf link without weasyprint:\n%s", body)
	}
	rec := get(h, "/index.md?pdf")
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), "weasyprint is not installed; install it to save a page as PDF") {
		t.Errorf("?pdf without weasyprint: %d\n%s", rec.Code, rec.Body)
	}
	// ?pdf on a code page is the code page.
	if rec := get(h, "/a/b.go?pdf"); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") == "application/pdf" {
		t.Errorf("?pdf on code: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

// A share offers no PDF, since each visitor would start weasyprint.
func TestSharedPageOffersNoPDF(t *testing.T) {
	s, tr, _ := serveTree(t)
	s.shared, tr.shared = true, true
	useWeasyprint(t, "/usr/bin/weasyprint")
	h := s.handler()
	if body := get(h, "/index.md").Body.String(); strings.Contains(body, `href="?pdf"`) {
		t.Errorf("a share links a PDF:\n%s", body)
	}
	if rec := get(h, "/index.md?pdf"); rec.Code != http.StatusForbidden {
		t.Errorf("?pdf on a share: %d", rec.Code)
	}
}

// interpreter reads the program a script's first line names.
func TestInterpreter(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		first string
		want  []string
	}{
		{"#!/usr/bin/python3\n", []string{"/usr/bin/python3"}},
		{"#! /usr/bin/env python3\n", []string{"/usr/bin/env", "python3"}},
		{"\x7fELF", nil},
	} {
		p := filepath.Join(dir, "w")
		os.WriteFile(p, []byte(c.first+"import x\n"), 0o755)
		got, err := interpreter(p)
		if strings.Join(got, " ") != strings.Join(c.want, " ") || (c.want == nil) != (err != nil) {
			t.Errorf("%q: %q, %v", c.first, got, err)
		}
	}
}

// weasyprint draws the page from www's own HTML and fetches through this
// server alone: a doc's image from the tree arrives, while an image on
// another host and a file: URL are never fetched.
func TestPDFDrawsThroughThisServerAlone(t *testing.T) {
	prog, err := exec.LookPath("weasyprint")
	if err != nil {
		t.Skip("weasyprint is not installed")
	}
	interp, err := interpreter(prog)
	if err != nil {
		t.Skip(err)
	}
	if out, err := exec.Command(interp[0], append(interp[1:], "-I", "-c", "import weasyprint")...).CombinedOutput(); err != nil {
		t.Skipf("weasyprint's module does not load: %v\n%s", err, out)
	}
	var stranger atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { stranger.Add(1) }))
	defer other.Close()

	s, _, dir := serveTree(t)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89" +
		"\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x35\x81\x84\x00\x00\x00\x00IEND\xaeB`\x82")
	os.WriteFile(filepath.Join(dir, "dot.png"), png, 0o644)
	os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# A doc\n\nSome text.\n\n![here](dot.png)\n\n![away]("+other.URL+"/x.png)\n\n![file](file:///etc/hostname)\n"), 0o644)
	var mine atomic.Int32
	h := s.handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/dot.png") {
			mine.Add(1)
		}
		h.ServeHTTP(w, r)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/doc.md?pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("?pdf: %d %s\n%.300s", resp.StatusCode, resp.Header.Get("Content-Type"), b)
	}
	if got := resp.Header.Get("Content-Disposition"); got != "attachment; filename*=UTF-8''doc.pdf" {
		t.Errorf("Content-Disposition %q", got)
	}
	if mine.Load() == 0 {
		t.Error("the tree's image was not fetched")
	}
	if n := stranger.Load(); n != 0 {
		t.Errorf("weasyprint fetched from another host %d times", n)
	}
}
