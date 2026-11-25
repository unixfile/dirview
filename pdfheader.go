package dirview

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"
)

// pdfScreenLine is the header a viewer shows on each page of a saved PDF:
// the command that saves the embedded source.
const pdfScreenLine = "pdfdetach -saveall"

// The header's place and look: Courier at 4.5 pt, the mono font every
// viewer has without one embedded, set flush right with the text, whose
// side margins typstPreamble sets at 47 mm. Its baseline is 10 mm below
// the page's top edge, since printers cannot print the outer 4 to 6 mm.
// The screen line is a lighter gray than the print line, which a printer
// breaks into dots, so the print line stays legible on paper.
const (
	pdfMM              = 72 / 25.4
	pdfHeaderSide      = 47 * pdfMM
	pdfHeaderTop       = 10 * pdfMM
	pdfHeaderSize      = 4.5
	pdfHeaderAdvance   = 0.6 // Courier's width per character, in ems
	pdfHeaderGray      = 0.75
	pdfHeaderPrintGray = 0.6
)

// Annotation flags, from table 165 of ISO 32000-1. A viewer shows an
// annotation without NoView and prints one with Print; Locked keeps a
// reader from moving or deleting it.
const (
	annotPrint  = 4
	annotNoView = 32
	annotLocked = 128
)

// pdfPrintLine names the source on paper: the repo, the file's path in it
// and git's short hash of the commit, eg u/r/doc/a.md@a1b2c3d, with -dirty
// where the file differs from that commit. A repo without a URL leaves
// out the repo, and outside a repo it is the file's name.
func pdfPrintLine(name string, keys []pdfKey) string {
	k := map[string]string{}
	for _, p := range keys {
		k[p.name] = p.value
	}
	commit := k["GitCommit"]
	if commit == "" {
		return path.Base(name)
	}
	line := k["GitPath"]
	if repo := repoName(k["GitURL"]); repo != "" {
		line = repo + "/" + line
	}
	line += "@" + commit[:min(7, len(commit))]
	if k["GitDirty"] == "true" {
		line += "-dirty"
	}
	return line
}

// repoName is a repo's path on its host, without .git, eg u/r for
// https://github.com/u/r.git or git@github.com:u/r. A URL without a host,
// eg /srv/git/r.git, keeps its whole path.
func repoName(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		_, u, _ = strings.Cut(u[i+3:], "/")
	} else if host, p, ok := strings.Cut(u, ":"); ok && !strings.Contains(host, "/") {
		u = p
	}
	return strings.TrimSuffix(strings.TrimSuffix(u, "/"), ".git")
}

// header stamps two lines at the top right of each page as watermark
// annotations, which lie outside the page's content, so the layout
// stays. screen shows in a viewer and does not print; print prints and
// does not show, where the viewer honours the flags. Each line is marked
// an artifact, for readers of tagged PDF, and a span whose actual text
// is a space, since poppler, eg pdftotext, reads annotations and skips
// no artifact; poppler 22.12 takes an empty actual text for none.
func (u *pdfUpdate) header(screen, print string) error {
	_, _, root, err := u.f.dict(u.f.trailer.get("/Root"))
	if err != nil {
		return err
	}
	font := u.add([]byte("<<\n  /Type /Font\n  /Subtype /Type1\n  /BaseFont /Courier\n  /Encoding /WinAnsiEncoding\n>>"))
	// Pages of one width share each line's appearance.
	forms := map[string]string{}
	form := func(width float64, text string, gray float64) string {
		key := pdfNum(width) + "\x00" + text
		if ref, ok := forms[key]; ok {
			return ref
		}
		text = winAnsi(text)
		x := max(0, width-float64(len(text))*pdfHeaderAdvance*pdfHeaderSize)
		draw := fmt.Sprintf("/Artifact BMC /Span << /ActualText ( ) >> BDC q %s g BT /F %s Tf %s 2 Td %s Tj ET Q EMC EMC",
			pdfNum(gray), pdfNum(pdfHeaderSize), pdfNum(x), pdfLiteral(text))
		forms[key] = u.add(fmt.Appendf(nil, "<<\n  /Type /XObject\n  /Subtype /Form\n  /BBox [0 0 %s 8]\n  /Resources << /Font << /F %s >> >>\n  /Length %d\n>>\nstream\n%s\nendstream",
			pdfNum(width), font, len(draw), draw))
		return forms[key]
	}
	var walk func(ref, box string, depth int) error
	walk = func(ref, box string, depth int) error {
		if depth > 64 {
			return fmt.Errorf("%w: the page tree is over 64 levels deep", errPDFForm)
		}
		num, gen, d, err := u.f.dict(ref)
		if err != nil {
			return err
		}
		if b := d.get("/MediaBox"); b != "" {
			box = b
		}
		if d.get("/Type") == "/Pages" {
			kids, ok := pdfRefs(d.get("/Kids"))
			if !ok {
				return fmt.Errorf("%w: object %d lists no kids", errPDFForm, num)
			}
			for _, kid := range kids {
				if err := walk(kid, box, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		x0, _, x1, y1, ok := pdfBox(box)
		if !ok {
			return fmt.Errorf("%w: page %d has no media box", errPDFForm, num)
		}
		side := pdfHeaderSide
		if x1-x0-2*side < 72 {
			side = pdfHeaderTop
		}
		left, right, base := x0+side, x1-side, y1-pdfHeaderTop
		rect := fmt.Sprintf("[%s %s %s %s]", pdfNum(left), pdfNum(base-2), pdfNum(right), pdfNum(base+6))
		var refs []string
		for _, l := range []struct {
			text  string
			flags int
			gray  float64
		}{{screen, annotLocked, pdfHeaderGray}, {print, annotPrint | annotNoView | annotLocked, pdfHeaderPrintGray}} {
			refs = append(refs, u.add(fmt.Appendf(nil, "<<\n  /Type /Annot\n  /Subtype /Watermark\n  /Rect %s\n  /F %d\n  /P %d %d R\n  /AP << /N %s >>\n>>",
				rect, l.flags, num, gen, form(right-left, l.text, l.gray))))
		}
		added := strings.Join(refs, " ")
		switch old := strings.TrimSpace(d.get("/Annots")); {
		case old == "":
			d.set("/Annots", "["+added+"]")
		case strings.HasPrefix(old, "["):
			d.set("/Annots", strings.TrimSuffix(old, "]")+" "+added+"]")
		default:
			n, g, text, err := u.f.object(old)
			if err != nil {
				return err
			}
			if !strings.HasPrefix(text, "[") {
				return fmt.Errorf("%w: page %d's annotations are no array", errPDFForm, num)
			}
			u.set(n, g, []byte(strings.TrimSuffix(text, "]")+" "+added+"]"))
		}
		u.set(num, gen, d.bytes())
		return nil
	}
	return walk(root.get("/Pages"), "", 0)
}

// pdfBox reads a rectangle, eg "[0 0 595.2756 841.8898]".
func pdfBox(v string) (x0, y0, x1, y1 float64, ok bool) {
	v = strings.TrimSpace(v)
	f := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(v, "["), "]"))
	if len(f) != 4 || !strings.HasPrefix(v, "[") {
		return 0, 0, 0, 0, false
	}
	var n [4]float64
	for i, s := range f {
		var err error
		if n[i], err = strconv.ParseFloat(s, 64); err != nil {
			return 0, 0, 0, 0, false
		}
	}
	return min(n[0], n[2]), min(n[1], n[3]), max(n[0], n[2]), max(n[1], n[3]), true
}

// pdfNum writes a number as a PDF does, to a thousandth of a point.
func pdfNum(v float64) string {
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
}

// winAnsi is s in the Windows code page Helvetica is set in: Latin-1
// stays, and each other character becomes a question mark.
func winAnsi(s string) string {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 0x20 && r <= 0x7e, r >= 0xa0 && r <= 0xff:
			b = append(b, byte(r))
		default:
			b = append(b, '?')
		}
	}
	return string(b)
}
