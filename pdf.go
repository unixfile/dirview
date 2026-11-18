package www

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

// pdfQuery is the query that asks for a Markdown page as PDF.
const pdfQuery = "pdf"

// pdfTimeout bounds one run of weasyprint; a long doc takes seconds.
const pdfTimeout = time.Minute

// maxPDF bounds the PDF one page may produce.
const maxPDF = 64 << 20

// weasyprint finds the weasyprint program, or fails when it is not
// installed. Tests replace it.
var weasyprint = func() (string, error) { return exec.LookPath("weasyprint") }

// pdfScript runs in weasyprint's own Python. It reads the page on stdin
// and writes the PDF to stdout. Its fetcher takes only data: URLs and
// URLs of the server that asked, as the page's CSP lets a browser take
// nothing else: a doc's image on another host, or a file: URL, stays out.
const pdfScript = `import sys, weasyprint
origin, base = sys.argv[1], sys.argv[2]
def fetch(url, *args, **kwargs):
    if url.startswith("data:") or url.startswith(origin + "/"):
        return weasyprint.default_url_fetcher(url, *args, **kwargs)
    raise ValueError("www fetches nothing beyond its own server: " + url)
weasyprint.HTML(string=sys.stdin.buffer.read().decode(), base_url=base, url_fetcher=fetch).write_pdf(sys.stdout.buffer)
`

// pdfPage is the HTML weasyprint draws: the page's body under the
// stylesheet, without the header, the search row, scripts or buttons,
// on A4 pages.
var pdfPage = template.Must(template.New("pdf").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.Title}}</title>
<link rel="stylesheet" href="{{.Style}}">
<style>@page { size: A4; margin: 2cm 2.2cm; } body { margin: 0; max-width: none; }</style>
</head>
<body class="doc">
<main>
{{.Body}}
</main>
</body>
</html>
`))

// pdfOffered reports whether t's Markdown pages offer a PDF: only on the
// reader's own servers, never on a share, where each visitor would start
// weasyprint, and only when weasyprint is installed.
func (t *tree) pdfOffered() bool {
	if t.shared || t.public || t.srv == nil || t.srv.pdf == nil {
		return false
	}
	_, err := weasyprint()
	return err == nil
}

// servePDF answers a Markdown page as PDF, drawn by weasyprint from the
// page's own HTML and stylesheet, as a file to save.
func (s *server) servePDF(w http.ResponseWriter, r *http.Request, t *tree, name string) {
	if t.shared || t.public {
		t.fail(w, r, name, http.StatusForbidden, "A shared page offers no PDF")
		return
	}
	prog, err := weasyprint()
	if err != nil {
		t.fail(w, r, name, http.StatusNotImplemented, "weasyprint is not installed; install it to save a page as PDF")
		return
	}
	b, err := fs.ReadFile(t.fsys, name)
	if err != nil {
		t.fail(w, r, name, http.StatusInternalServerError, err.Error())
		return
	}
	body, err := renderMarkdown(b, t.missing(name), t.webBase())
	if err != nil {
		t.fail(w, r, name, http.StatusInternalServerError, err.Error())
		return
	}
	var page bytes.Buffer
	if err := pdfPage.Execute(&page, struct {
		Title, Style string
		Body         template.HTML
	}{t.title(name), styleURL, body}); err != nil {
		t.fail(w, r, name, http.StatusInternalServerError, err.Error())
		return
	}
	origin := linkOrigin(r)
	start := time.Now()
	pdf, err := drawPDF(r.Context(), prog, origin, origin+r.URL.EscapedPath(), page.Bytes())
	if err != nil {
		logErr("E%d: cannot draw %s as PDF: %v", errnoOf(err), name, err)
		t.fail(w, r, name, http.StatusInternalServerError, "weasyprint failed: "+err.Error())
		return
	}
	logInfo("drew %s as PDF in %d ms, %s", name, time.Since(start).Milliseconds(), sizeText(int64(len(pdf))))
	file := strings.TrimSuffix(path.Base(name), path.Ext(name)) + ".pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(file))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(pdf)
}

// drawPDF runs weasyprint's Python on page, with relative URLs taken from
// base. weasyprint is a Python program, so the script runs under the
// interpreter its first line names, which holds its module.
func drawPDF(ctx context.Context, prog, origin, base string, page []byte) ([]byte, error) {
	interp, err := interpreter(prog)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, pdfTimeout)
	defer cancel()
	args := append(interp[1:], "-I", "-c", pdfScript, origin, base)
	cmd := exec.CommandContext(ctx, interp[0], args...)
	// No proxy variables: the fetcher reaches only this server. fontconfig
	// keeps its cache under home.
	for _, k := range []string{"PATH", "HOME", "XDG_CACHE_HOME", "LANG"} {
		if v, ok := os.LookupEnv(k); ok {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	cmd.Stdin = bytes.NewReader(page)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &limited{&out, maxPDF}, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("weasyprint took over %s", pdfTimeout)
		}
		msg := lastLine(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return out.Bytes(), nil
}

// interpreter is the command in the first line of the script at prog, eg
// /usr/bin/python3, or /usr/bin/env python3.
func interpreter(prog string) ([]string, error) {
	f, err := os.Open(prog)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	line, _ := bufio.NewReader(f).ReadString('\n')
	cmd, ok := strings.CutPrefix(strings.TrimSpace(line), "#!")
	if f := strings.Fields(cmd); ok && len(f) > 0 {
		return f, nil
	}
	return nil, errors.New(prog + " is not a Python script, so www cannot run its module")
}

// lastLine is the last line of s that holds text: a Python traceback
// ends with the error.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
