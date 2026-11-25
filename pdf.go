package dirview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// pdfQuery is the query that asks for a Markdown page as PDF.
const pdfQuery = "pdf"

// pdfTimeout bounds one run of typst; a long doc takes a second or two.
const pdfTimeout = time.Minute

// maxPDF bounds the PDF one page may produce, and the images it may take.
const maxPDF = 64 << 20

// typstMissing says how to get typst, which no Debian suite packages.
const typstMissing = "typst is not installed; install it from https://github.com/typst/typst/releases to save a page as PDF"

// typst finds the typst program, or fails when it is not installed. Tests
// replace it.
var typst = func() (string, error) { return exec.LookPath("typst") }

// pdfOffered reports whether t's Markdown pages offer a PDF: only on the
// reader's own servers, never on a share, where each visitor would start
// typst, and only when typst is installed.
func (t *tree) pdfOffered() bool {
	if t.shared || t.public || t.srv == nil || t.srv.pdf == nil {
		return false
	}
	_, err := typst()
	return err == nil
}

// servePDF answers a Markdown page as PDF, typeset by typst from drvu's
// own reading of the doc, as a file to save.
func (s *server) servePDF(w http.ResponseWriter, r *http.Request, t *tree, name string) {
	if t.shared || t.public {
		t.fail(w, r, name, http.StatusForbidden, "A shared page offers no PDF")
		return
	}
	prog, err := typst()
	if err != nil {
		t.fail(w, r, name, http.StatusNotImplemented, typstMissing)
		return
	}
	src, err := fs.ReadFile(t.fsys, name)
	if err != nil {
		t.fail(w, r, name, http.StatusInternalServerError, err.Error())
		return
	}
	page, err := url.Parse(linkOrigin(r) + r.URL.EscapedPath())
	if err != nil {
		t.fail(w, r, name, http.StatusInternalServerError, err.Error())
		return
	}
	start := time.Now()
	pdf, err := t.typesetPDF(r.Context(), prog, name, src, page)
	if err != nil {
		logErr("E%d: cannot typeset %s as PDF: %v", errnoOf(err), name, err)
		t.fail(w, r, name, http.StatusInternalServerError, "typst failed: "+err.Error())
		return
	}
	// The embedded source is the truth, so a PDF whose keys could not be
	// written still goes out.
	if out, err := pdfInfo(pdf, t.pdfKeys(r.Context(), name, src, time.Now())); err != nil {
		logErr("E%d: cannot write the source's keys into the PDF of %s: %v", errnoOf(err), name, err)
	} else {
		pdf = out
	}
	logInfo("typeset %s as PDF in %d ms, %s", name, time.Since(start).Milliseconds(), sizeText(int64(len(pdf))))
	file := strings.TrimSuffix(path.Base(name), path.Ext(name)) + ".pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(file))
	w.Header().Set("Cache-Control", "no-store")
	w.Write(pdf)
}

// typesetPDF writes the doc name, whose text is src, as typst markup in a
// directory of its own, with the doc's images from the tree beside it,
// and runs typst there. The PDF embeds src, byte for byte, under the
// doc's own file name. typst reads nothing outside that directory, so
// the PDF holds only what the tree gives, as the page's CSP lets a
// browser take nothing else. An image typst cannot read fails the whole
// run, so a failed run with images is tried again without them, as a
// browser shows a page whose image is broken.
func (t *tree) typesetPDF(ctx context.Context, prog, name string, src []byte, page *url.URL) ([]byte, error) {
	dir, err := os.MkdirTemp("", progName+"-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, typstSource), src, 0o600); err != nil {
		return nil, err
	}
	attach := typstAttach(path.Base(name))
	images := t.pdfImages(dir, name)
	pdf, err := runTypst(ctx, prog, dir, append([]byte(attach), typstDoc(src, t.title(name), page, t.missing(name), images.file)...))
	if err != nil && images.count > 0 && ctx.Err() == nil {
		logErr("E%d: cannot typeset %s with its images, so it goes without: %v", errnoOf(err), name, err)
		pdf, err = runTypst(ctx, prog, dir, append([]byte(attach), typstDoc(src, t.title(name), page, t.missing(name), nil)...))
	}
	return pdf, err
}

// pdfImageSet copies a doc's images from the tree into typst's directory.
type pdfImageSet struct {
	t     *tree
	dir   string
	doc   string            // the doc's path in the tree
	files map[string]string // tree path to file name, or "" for one left out
	count int
	size  int64
}

func (t *tree) pdfImages(dir, doc string) *pdfImageSet {
	return &pdfImageSet{t: t, dir: dir, doc: doc, files: map[string]string{}}
}

// file is the name in typst's directory of the image at dest, or "" for
// one the PDF goes without: an image on another host or outside the tree,
// one the tree refuses to serve, eg on a path the guard keeps out, one in
// a format typst does not read, and any past maxPDF in all.
func (s *pdfImageSet) file(dest string) string {
	rel := relativePath(dest)
	if rel == "" {
		return ""
	}
	p := path.Join(path.Dir(s.doc), rel)
	if !fs.ValidPath(p) {
		return ""
	}
	if f, ok := s.files[p]; ok {
		return f
	}
	s.files[p] = ""
	info, err := fs.Stat(s.t.fsys, p)
	if err != nil || !info.Mode().IsRegular() || s.size+info.Size() > maxPDF {
		return ""
	}
	b, err := fs.ReadFile(s.t.fsys, p)
	if err != nil || s.size+int64(len(b)) > maxPDF {
		return ""
	}
	ext := imageFormat(p, b)
	if ext == "" {
		return ""
	}
	f := "image" + strconv.Itoa(s.count+1) + "." + ext
	if err := os.WriteFile(filepath.Join(s.dir, f), b, 0o600); err != nil {
		return ""
	}
	s.files[p] = f
	s.count++
	s.size += int64(len(b))
	return f
}

// imageFormat is the extension typst knows an image by, read from its
// first bytes, or "" for a format typst does not read. SVG has no magic
// number, so it goes by its name and the svg element in its text.
func imageFormat(name string, b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case bytes.HasPrefix(b, []byte("\xff\xd8\xff")):
		return "jpg"
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return "gif"
	case len(b) >= 12 && bytes.HasPrefix(b, []byte("RIFF")) && string(b[8:12]) == "WEBP":
		return "webp"
	case strings.EqualFold(path.Ext(name), ".svg") && isText(b) && bytes.Contains(b, []byte("<svg")):
		return "svg"
	}
	return ""
}

// runTypst typesets markup in dir and returns the PDF. typst reads only
// dir, and fonts. It prints a warning, eg for a font it lacks, even when
// it succeeds; each goes to the log.
func runTypst(ctx context.Context, prog, dir string, markup []byte) ([]byte, error) {
	in := filepath.Join(dir, "page.typ")
	if err := os.WriteFile(in, markup, 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, pdfTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, prog, "compile", "--root", dir, "--format", "pdf", "--diagnostic-format", "short", in, "-")
	cmd.Dir = dir
	// No proxy variables, though the markup imports no package that
	// typst could fetch. fontconfig finds the fonts under home through the
	// rest.
	for _, k := range []string{"HOME", "XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "LANG"} {
		if v, ok := os.LookupEnv(k); ok {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &limited{&out, maxPDF}, &stderr
	err := cmd.Run()
	var errs []string
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, dir+string(filepath.Separator), ""))
		switch {
		case strings.Contains(line, ": warning: "):
			logInfo("typst: %s", line)
		case strings.Contains(line, ": error: "):
			errs = append(errs, line)
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("typst took over %s", pdfTimeout)
		}
		if len(errs) == 0 {
			return nil, err
		}
		return nil, errors.New(strings.Join(errs, "; "))
	}
	return out.Bytes(), nil
}
