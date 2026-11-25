package dirview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// useTypst makes typst report prog as installed, or missing when prog is
// "".
func useTypst(t *testing.T, prog string) {
	t.Helper()
	old := typst
	typst = func() (string, error) {
		if prog == "" {
			return "", errors.New("not found")
		}
		return prog, nil
	}
	t.Cleanup(func() { typst = old })
}

// A Markdown page links its PDF beside raw only while typst is installed.
// Without it, ?pdf says what is missing and where to get it. A code page
// has no PDF.
func TestPDFLinkFollowsTypst(t *testing.T) {
	s, _, _ := serveTree(t)
	h := s.handler()
	link := `<a href="?pdf" title="save this page as PDF, typeset by typst">pdf</a>`

	useTypst(t, "/usr/bin/typst")
	if body := get(h, "/index.md").Body.String(); !strings.Contains(body, `<a href="?raw">raw</a>`+link) {
		t.Errorf("no pdf link beside raw:\n%s", body)
	}
	if body := get(h, "/a/b.go").Body.String(); strings.Contains(body, link) {
		t.Errorf("a code page links a PDF:\n%s", body)
	}

	useTypst(t, "")
	if body := get(h, "/index.md").Body.String(); strings.Contains(body, link) {
		t.Errorf("pdf link without typst:\n%s", body)
	}
	rec := get(h, "/index.md?pdf")
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), "typst is not installed; install it from https://github.com/typst/typst/releases to save a page as PDF") {
		t.Errorf("?pdf without typst: %d\n%s", rec.Code, rec.Body)
	}
	// ?pdf on a code page is the code page.
	if rec := get(h, "/a/b.go?pdf"); rec.Code != http.StatusOK || rec.Header().Get("Content-Type") == "application/pdf" {
		t.Errorf("?pdf on code: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

// A share offers no PDF, since each visitor would start typst.
func TestSharedPageOffersNoPDF(t *testing.T) {
	s, tr, _ := serveTree(t)
	s.shared, tr.shared = true, true
	useTypst(t, "/usr/bin/typst")
	h := s.handler()
	if body := get(h, "/index.md").Body.String(); strings.Contains(body, `href="?pdf"`) {
		t.Errorf("a share links a PDF:\n%s", body)
	}
	if rec := get(h, "/index.md?pdf"); rec.Code != http.StatusForbidden {
		t.Errorf("?pdf on a share: %d", rec.Code)
	}
}

// onePixel is a PNG of one pixel.
var onePixel = func() []byte {
	var b bytes.Buffer
	png.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1)))
	return b.Bytes()
}()

// needTypst skips a test where typst is not installed.
func needTypst(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst is not installed")
	}
}

// pdfOf asks h for path as PDF and fails unless a PDF comes back.
func pdfOf(t *testing.T, h http.Handler, path string) []byte {
	t.Helper()
	rec := get(h, path+"?pdf")
	b := rec.Body.Bytes()
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("%s?pdf: %d %s\n%.400s", path, rec.Code, rec.Header().Get("Content-Type"), b)
	}
	return b
}

// pdfTool runs a poppler tool on pdf, named in args by "{}", or skips
// where the tool is missing.
func pdfTool(t *testing.T, pdf []byte, tool string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("%s is not installed", tool)
	}
	f := filepath.Join(t.TempDir(), "doc.pdf")
	os.WriteFile(f, pdf, 0o644)
	args = slices.Clone(args)
	args[slices.Index(args, "{}")] = f
	out, err := exec.Command(tool, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", tool, err, out)
	}
	return string(out)
}

// Every doc in testdata/typst typesets, and the PDF reads as the doc: each
// mark typst would take as markup arrives as text.
func TestPDFTypesets(t *testing.T) {
	needTypst(t)
	s, _, dir := serveTree(t)
	os.MkdirAll(filepath.Join(dir, "notes"), 0o755)
	docs, _ := filepath.Glob(filepath.Join("testdata", "typst", "*.md"))
	for _, doc := range docs {
		b, _ := os.ReadFile(doc)
		os.WriteFile(filepath.Join(dir, "notes", filepath.Base(doc)), b, 0o644)
	}
	os.WriteFile(filepath.Join(dir, "notes", "dot.png"), onePixel, 0o644)
	h := s.handler()
	for _, doc := range docs {
		pdfOf(t, h, "/notes/"+filepath.Base(doc))
	}
	if got := get(h, "/notes/text.md?pdf").Header().Get("Content-Disposition"); got != "attachment; filename*=UTF-8''text.pdf" {
		t.Errorf("Content-Disposition %q", got)
	}
	words := strings.Join(strings.Fields(pdfTool(t, pdfOf(t, h, "/notes/text.md"), "pdftotext", "{}", "-")), " ")
	for _, want := range []string{
		`! " # $ % & ’ ( ) * + , - . / : ; < = > ? @ [ \ ] ^ _ ` + "`" + ` { | } ~ and *escaped*.`,
		"“Quotes” and ‘single’, a – dash, a — rule and dots…",
		"© & A B \uFFFD &nosuch; AT&T",
		"= not a heading, - not a list, + not one, / not a term 1. starts a line",
		"A #hash, $math$, @ref, <label>, // comment and https://example.org/ in text.",
	} {
		if !strings.Contains(words, want) {
			t.Errorf("the PDF lacks %q:\n%s", want, words)
		}
	}
}

// The PDF takes images from the tree alone, each once however often the
// doc shows it. An image typst cannot read leaves the PDF without images
// rather than without a PDF.
func TestPDFImagesComeFromTheTree(t *testing.T) {
	needTypst(t)
	s, _, dir := serveTree(t)
	os.MkdirAll(filepath.Join(dir, "notes"), 0o755)
	os.WriteFile(filepath.Join(dir, "notes", "dot.png"), onePixel, 0o644)
	os.WriteFile(filepath.Join(dir, "notes", "bad.png"), []byte("\x89PNG\r\n\x1a\nnot a png"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes", "fake.png"), []byte("plain text"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes", "two.md"), []byte("![a](dot.png) ![b](dot.png) ![fake](fake.png) ![rooted](/etc/x.png) ![outside](../../x.png)\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes", "bad.md"), []byte("![a](dot.png) and ![broken](bad.png)\n"), 0o644)
	h := s.handler()

	pdf := pdfOf(t, h, "/notes/two.md")
	if shown, stored := pdfImages(t, pdf); shown != 2 || stored != 1 {
		t.Errorf("%d images shown from %d stored, want 2 from 1", shown, stored)
	}
	if words := strings.Fields(pdfTool(t, pdf, "pdftotext", "{}", "-")); strings.Join(words, " ") != "fake rooted outside" {
		t.Errorf("the descriptions of images left out: %q", words)
	}
	pdf = pdfOf(t, h, "/notes/bad.md")
	if shown, _ := pdfImages(t, pdf); shown != 0 {
		t.Errorf("%d images in a PDF whose image is broken", shown)
	}
	if words := strings.Fields(pdfTool(t, pdf, "pdftotext", "{}", "-")); strings.Join(words, " ") != "a and broken" {
		t.Errorf("the descriptions: %q", words)
	}
}

// pdfImages counts the images a PDF shows, and the image objects it
// stores, as pdfimages lists them.
func pdfImages(t *testing.T, pdf []byte) (shown, stored int) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(pdfTool(t, pdf, "pdfimages", "-list", "{}")), "\n")
	objects := map[string]bool{}
	for _, line := range lines[2:] {
		// page num type width height color comp bpc enc interp object ID ...
		if f := strings.Fields(line); len(f) > 11 {
			objects[f[10]+" "+f[11]] = true
		}
	}
	return len(lines) - 2, len(objects)
}

// imageFormat reads an image's format from its bytes, and SVG from its
// name and text.
func TestImageFormat(t *testing.T) {
	for _, c := range []struct {
		name string
		b    []byte
		want string
	}{
		{"a.png", onePixel, "png"},
		{"a.jpeg", []byte("\xff\xd8\xff\xe0rest"), "jpg"},
		{"a", []byte("GIF89a..."), "gif"},
		{"a.gif", []byte("GIF87a..."), "gif"},
		{"a.webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "webp"},
		{"a.svg", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), "svg"},
		{"A.SVG", []byte(`<svg/>`), "svg"},
		{"a.txt", []byte(`<svg/>`), ""},
		{"a.svg", []byte("<svg\x00"), ""},
		{"a.png", []byte("plain"), ""},
		{"a.bmp", []byte("BM...."), ""},
	} {
		if got := imageFormat(c.name, c.b); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.name, c.b, got, c.want)
		}
	}
}

// pdfKeysOf reads the custom keys of a PDF's document info, as pdfinfo
// lists them.
func pdfKeysOf(t *testing.T, pdf []byte) map[string]string {
	t.Helper()
	keys := map[string]string{}
	for _, line := range strings.Split(pdfTool(t, pdf, "pdfinfo", "-custom", "{}"), "\n") {
		if k, v, ok := strings.Cut(line, ":"); ok {
			keys[k] = strings.TrimSpace(v)
		}
	}
	return keys
}

// A saved PDF embeds one file, the doc's source byte for byte under its
// own name as Markdown, and names where the source lies in git, its
// SHA-256, when drvu typeset it and which build did. origin's password
// stays out.
func TestPDFCarriesItsSource(t *testing.T) {
	needTypst(t)
	s, dir := repoTree(t)
	name := "Ärende \"q\" (1).md"
	src := []byte("# Ärende\r\n\r\nA line with CRLF and a tab\there.\n")
	os.WriteFile(filepath.Join(dir, "a", name), src, 0o644)
	commitAll(t, dir, "Add the case")
	if out, err := exec.Command("git", "-C", dir, "remote", "add", "origin", "https://u:secret@example.org/r.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v\n%s", err, out)
	}
	h := s.handler()
	pdf := pdfOf(t, h, "/a/"+url.PathEscape(name))

	if got := pdfTool(t, pdf, "pdfdetach", "-list", "{}"); got != "1 embedded files\n1: "+name+"\n" {
		t.Errorf("attachments:\n%s", got)
	}
	out := t.TempDir()
	pdfTool(t, pdf, "pdfdetach", "-saveall", "-o", out, "{}")
	if got, err := os.ReadFile(filepath.Join(out, name)); err != nil || !bytes.Equal(got, src) {
		t.Errorf("the embedded file: %q, %v, want %q", got, err, src)
	}
	if !bytes.Contains(pdf, []byte("/Subtype /text#2Fmarkdown")) {
		t.Errorf("the embedded file is not text/markdown")
	}
	pdfTool(t, pdf, "qpdf", "--check", "{}")

	sum := sha256.Sum256(src)
	keys := pdfKeysOf(t, pdf)
	for k, want := range map[string]string{
		"GitURL":           "https://example.org/r.git",
		"GitCommit":        head(t, dir, "HEAD"),
		"GitPath":          "a/" + name,
		"GitDirty":         "false",
		"SourceSHA256":     hex.EncodeToString(sum[:]),
		"GeneratorVersion": progName + " " + buildCommit,
		"Title":            "a/" + name,
		"Creator":          keys["Creator"],
	} {
		if keys[k] != want || want == "" {
			t.Errorf("%s: %q, want %q", k, keys[k], want)
		}
	}
	if _, err := time.Parse(time.RFC3339, keys["Generated"]); err != nil {
		t.Errorf("Generated: %v", err)
	}
	if bytes.Contains(pdf, []byte("secret")) {
		t.Errorf("origin's password is in the PDF")
	}

	os.WriteFile(filepath.Join(dir, "index.md"), []byte("# Changed\n"), 0o644)
	if keys := pdfKeysOf(t, pdfOf(t, h, "/index.md")); keys["GitDirty"] != "true" || keys["GitPath"] != "index.md" {
		t.Errorf("a changed doc: %v", keys)
	}
}

// Outside a repo the PDF names no git, but still its source's sum.
func TestPDFOutsideARepo(t *testing.T) {
	needTypst(t)
	s, _, _ := serveTree(t)
	keys := pdfKeysOf(t, pdfOf(t, s.handler(), "/index.md"))
	for _, k := range []string{"GitURL", "GitCommit", "GitPath", "GitDirty"} {
		if v, ok := keys[k]; ok {
			t.Errorf("%s: %q outside a repo", k, v)
		}
	}
	if len(keys["SourceSHA256"]) != 64 {
		t.Errorf("SourceSHA256: %q", keys["SourceSHA256"])
	}
}

// pdfInfo refuses a PDF it cannot read rather than write a broken one,
// and writes text that is not plain ASCII as UTF-16.
func TestPDFInfo(t *testing.T) {
	const body = "%PDF-1.7\n1 0 obj\n<< /Title (a (b) >> c) /N [1 2] >>\nendobj\n" +
		"xref\n0 2\n0000000000 65535 f \n0000000009 00000 n \ntrailer\n<< /Size 2 /Root 1 0 R /Info 1 0 R >>\nstartxref\n"
	good := []byte(fmt.Sprintf("%s%d\n%%%%EOF\n", body, strings.Index(body, "xref")))
	out, err := pdfInfo(good, []pdfKey{{"K", "Ä (x)"}, {"L", `a\(b`}})
	if err != nil {
		t.Fatal(err)
	}
	want := "1 0 obj\n<< /Title (a (b) >> c) /N [1 2]\n  /K <FEFF00C40020002800780029>\n  /L (a\\\\\\(b)\n>>\nendobj\n"
	if added := string(out[len(good):]); !strings.HasPrefix(added, want) || !strings.Contains(added, "/Info 1 0 R\n  /Prev ") {
		t.Errorf("the update:\n%s", added)
	}
	if again, err := pdfInfo(out, []pdfKey{{"M", "m"}}); err != nil || !bytes.Contains(again, []byte("/K <FEFF")) {
		t.Errorf("a second update: %v\n%s", err, again)
	}
	for _, c := range []struct{ name, pdf string }{
		{"no startxref", "%PDF-1.7\n"},
		{"an xref stream", "%PDF-1.7\n1 0 obj\n<< /Type /XRef >>\nstream\nendstream\nendobj\nstartxref\n9\n%%EOF\n"},
		{"no info", strings.Replace(string(good), "/Info 1 0 R", "", 1)},
		{"a free info", strings.Replace(string(good), "0000000009 00000 n", "0000000009 00000 f", 1)},
		{"a lying table", strings.Replace(string(good), "0000000009", "0000000003", 1)},
	} {
		if _, err := pdfInfo([]byte(c.pdf), nil); !errors.Is(err, errPDFForm) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}
