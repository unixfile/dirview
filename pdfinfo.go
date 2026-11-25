package dirview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// pdfKey is one key drvu writes into a saved PDF's document info.
type pdfKey struct{ name, value string }

// pdfKeys are the keys a saved PDF carries beside its embedded source, as
// decision 305 lists them: where the source lies in git, as provenance
// only; the SHA-256 of the embedded file, which carries integrity; when
// drvu typeset it, and which build did.
func (t *tree) pdfKeys(ctx context.Context, name string, src []byte, now time.Time) []pdfKey {
	sum := sha256.Sum256(src)
	return append(t.gitKeys(ctx, name, src),
		pdfKey{"SourceSHA256", hex.EncodeToString(sum[:])},
		pdfKey{"Generated", now.Format(time.RFC3339)},
		pdfKey{"GeneratorVersion", progName + " " + buildCommit})
}

// gitKeys name the repo, the commit HEAD names, the file's path in the
// repo and whether the file differs from that commit; none outside a
// repo or in one without a commit. The URL is the forge's for a repo on
// the web address, else origin's without its user and password; a repo
// without origin has no URL.
func (t *tree) gitKeys(ctx context.Context, name string, src []byte) []pdfKey {
	dir, spec, err := t.gitPlace(name)
	if err != nil {
		return nil
	}
	r := repoAt(ctx, dir)
	if r.complaint != "" {
		return nil
	}
	commit := r.head(ctx)
	if !isHash(commit) {
		return nil
	}
	prefix, err := git(ctx, dir, "rev-parse", "--show-prefix")
	if err != nil {
		return nil
	}
	var keys []pdfKey
	if u := t.gitURL(ctx, dir); u != "" {
		keys = append(keys, pdfKey{"GitURL", u})
	}
	return append(keys,
		pdfKey{"GitCommit", commit},
		pdfKey{"GitPath", strings.TrimSuffix(string(prefix), "\n") + spec},
		pdfKey{"GitDirty", strconv.FormatBool(t.pageChanged(ctx, name, src))})
}

// gitURL is the URL of the repo dir lies in, or "". A token in origin's
// URL stays out of the PDF.
func (t *tree) gitURL(ctx context.Context, dir string) string {
	web := t.web
	if t.base != nil {
		web = t.base.web
	}
	if web != nil {
		return "https://" + web.key
	}
	out, err := git(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if u, err := url.Parse(s); err == nil && u.User != nil {
		u.User = nil
		s = u.String()
	}
	return s
}

// errPDFForm is the error for a PDF that pdfInfo cannot read.
var errPDFForm = errors.New("not a PDF with a cross-reference table and an info dictionary")

// pdfInfo adds keys to pdf's document info in an incremental update: the
// bytes typst wrote stay as they are, and a new version of the info
// dictionary, a cross-reference section and a trailer follow them. It
// reads the classic cross-reference table that typst writes, and fails
// on any other form.
func pdfInfo(pdf []byte, keys []pdfKey) ([]byte, error) {
	at := bytes.LastIndex(pdf, []byte("startxref"))
	if at < 0 {
		return nil, fmt.Errorf("%w: no startxref", errPDFForm)
	}
	xref, err := strconv.Atoi(pdfToken(pdf, at+len("startxref")))
	if err != nil || xref < 0 || xref >= len(pdf) {
		return nil, fmt.Errorf("%w: startxref names no offset", errPDFForm)
	}
	trailer, err := pdfTrailer(pdf, xref)
	if err != nil {
		return nil, err
	}
	num, gen, ok := pdfRef(trailer["/Info"])
	if !ok || trailer["/Root"] == "" {
		return nil, fmt.Errorf("%w: the trailer names no info or root", errPDFForm)
	}
	off, err := pdfObject(pdf, xref, num, gen)
	if err != nil {
		return nil, err
	}
	head := fmt.Sprintf("%d %d obj", num, gen)
	i := pdfSkip(pdf, off)
	if !bytes.HasPrefix(pdf[i:], []byte(head)) {
		return nil, fmt.Errorf("%w: object %d is not where the table says", errPDFForm, num)
	}
	start := pdfSkip(pdf, i+len(head))
	end, err := pdfValue(pdf, start)
	if err != nil || !bytes.HasPrefix(pdf[start:], []byte("<<")) {
		return nil, fmt.Errorf("%w: object %d holds no dictionary", errPDFForm, num)
	}

	out := bytes.NewBuffer(make([]byte, 0, len(pdf)+1024))
	out.Write(pdf)
	if !bytes.HasSuffix(pdf, []byte("\n")) {
		out.WriteByte('\n')
	}
	obj := out.Len()
	fmt.Fprintf(out, "%s\n", head)
	out.Write(bytes.TrimRightFunc(pdf[start:end-2], func(r rune) bool { return r < 0x80 && isPDFSpace(byte(r)) }))
	for _, k := range keys {
		fmt.Fprintf(out, "\n  /%s %s", k.name, pdfText(k.value))
	}
	out.WriteString("\n>>\nendobj\n")
	table := out.Len()
	fmt.Fprintf(out, "xref\n%d 1\n%010d %05d n \ntrailer\n<<\n  /Size %s\n  /Root %s\n  /Info %d %d R\n",
		num, obj, gen, trailer["/Size"], trailer["/Root"], num, gen)
	if id := trailer["/ID"]; id != "" {
		fmt.Fprintf(out, "  /ID %s\n", id)
	}
	fmt.Fprintf(out, "  /Prev %d\n>>\nstartxref\n%d\n%%%%EOF\n", xref, table)
	return out.Bytes(), nil
}

// pdfTrailer reads the trailer of the cross-reference section at xref.
func pdfTrailer(pdf []byte, xref int) (map[string]string, error) {
	i := pdfSkip(pdf, xref)
	if !bytes.HasPrefix(pdf[i:], []byte("xref")) {
		return nil, fmt.Errorf("%w: no table at startxref", errPDFForm)
	}
	at := bytes.Index(pdf[i:], []byte("trailer"))
	if at < 0 {
		return nil, fmt.Errorf("%w: no trailer", errPDFForm)
	}
	d, err := pdfDict(pdf, pdfSkip(pdf, i+at+len("trailer")))
	if err != nil {
		return nil, fmt.Errorf("%w: the trailer: %v", errPDFForm, err)
	}
	return d, nil
}

// pdfObject is the offset of object num in generation gen, from the table
// at xref or, failing that, the tables before it.
func pdfObject(pdf []byte, xref, num, gen int) (int, error) {
	for seen := map[int]bool{}; !seen[xref]; {
		seen[xref] = true
		i := pdfSkip(pdf, xref) + len("xref")
		for {
			i = pdfSkip(pdf, i)
			if bytes.HasPrefix(pdf[i:], []byte("trailer")) {
				break
			}
			first, err1 := strconv.Atoi(pdfToken(pdf, i))
			i = pdfSkip(pdf, i+len(pdfToken(pdf, i)))
			count, err2 := strconv.Atoi(pdfToken(pdf, i))
			i += len(pdfToken(pdf, i))
			// Each entry is 20 bytes after the line that counts them.
			for i < len(pdf) && (pdf[i] == ' ' || pdf[i] == '\r' || pdf[i] == '\n') {
				i++
			}
			if err1 != nil || err2 != nil || count < 0 || i+20*count > len(pdf) {
				return 0, fmt.Errorf("%w: a broken table", errPDFForm)
			}
			if num >= first && num < first+count {
				e := strings.Fields(string(pdf[i+20*(num-first) : i+20*(num-first+1)]))
				off, err := strconv.Atoi(e[0])
				if len(e) != 3 || e[2] != "n" || err != nil || e[1] != fmt.Sprintf("%05d", gen) {
					return 0, fmt.Errorf("%w: object %d is not in use", errPDFForm, num)
				}
				return off, nil
			}
			i += 20 * count
		}
		t, err := pdfTrailer(pdf, xref)
		if err != nil {
			return 0, err
		}
		if xref, err = strconv.Atoi(t["/Prev"]); err != nil {
			break
		}
	}
	return 0, fmt.Errorf("%w: no object %d", errPDFForm, num)
}

// pdfRef reads an indirect reference, eg "18 0 R".
func pdfRef(v string) (num, gen int, ok bool) {
	f := strings.Fields(v)
	if len(f) != 3 || f[2] != "R" {
		return 0, 0, false
	}
	num, err1 := strconv.Atoi(f[0])
	gen, err2 := strconv.Atoi(f[1])
	return num, gen, err1 == nil && err2 == nil
}

// pdfDict reads the dictionary at i into its keys and their values, as
// they stand in the file.
func pdfDict(b []byte, i int) (map[string]string, error) {
	if !bytes.HasPrefix(b[i:], []byte("<<")) {
		return nil, errors.New("no dictionary")
	}
	d := map[string]string{}
	i += 2
	for {
		i = pdfSkip(b, i)
		if i >= len(b) {
			return nil, errors.New("no end")
		}
		if bytes.HasPrefix(b[i:], []byte(">>")) {
			return d, nil
		}
		if b[i] != '/' {
			return nil, errors.New("a key that is not a name")
		}
		key := pdfToken(b, i)
		start := pdfSkip(b, i+len(key))
		end, err := pdfValue(b, start)
		if err != nil {
			return nil, err
		}
		// An indirect reference is three tokens: two numbers and R.
		if j := pdfSkip(b, end); isDigits(b[start:end]) && isDigits([]byte(pdfToken(b, j))) {
			if k := pdfSkip(b, j+len(pdfToken(b, j))); pdfToken(b, k) == "R" {
				end = k + 1
			}
		}
		d[key] = string(b[start:end])
		i = end
	}
}

// pdfValue is the end of the value that starts at i.
func pdfValue(b []byte, i int) (int, error) {
	if i >= len(b) {
		return 0, errors.New("no value")
	}
	switch {
	case bytes.HasPrefix(b[i:], []byte("<<")):
		for i += 2; ; {
			i = pdfSkip(b, i)
			if i >= len(b) {
				return 0, errors.New("an unended dictionary")
			}
			if bytes.HasPrefix(b[i:], []byte(">>")) {
				return i + 2, nil
			}
			end, err := pdfValue(b, i)
			if err != nil {
				return 0, err
			}
			i = end
		}
	case b[i] == '[':
		for i++; ; {
			i = pdfSkip(b, i)
			if i >= len(b) {
				return 0, errors.New("an unended array")
			}
			if b[i] == ']' {
				return i + 1, nil
			}
			end, err := pdfValue(b, i)
			if err != nil {
				return 0, err
			}
			i = end
		}
	case b[i] == '<':
		end := bytes.IndexByte(b[i:], '>')
		if end < 0 {
			return 0, errors.New("an unended hex string")
		}
		return i + end + 1, nil
	case b[i] == '(':
		depth := 0
		for j := i; j < len(b); j++ {
			switch b[j] {
			case '\\':
				j++
			case '(':
				depth++
			case ')':
				if depth--; depth == 0 {
					return j + 1, nil
				}
			}
		}
		return 0, errors.New("an unended string")
	}
	tok := pdfToken(b, i)
	if tok == "" {
		return 0, fmt.Errorf("an unexpected %q", b[i])
	}
	return i + len(tok), nil
}

// pdfToken is the name, number or keyword at i, up to the next blank or
// delimiter; a name keeps its slash.
func pdfToken(b []byte, i int) string {
	i = pdfSkip(b, i)
	j := i
	if j < len(b) && b[j] == '/' {
		j++
	}
	for j < len(b) && !isPDFSpace(b[j]) && !strings.ContainsRune("()<>[]{}/%", rune(b[j])) {
		j++
	}
	return string(b[i:j])
}

// pdfSkip is the first offset from i that is neither blank nor in a
// comment.
func pdfSkip(b []byte, i int) int {
	for i < len(b) {
		switch {
		case isPDFSpace(b[i]):
			i++
		case b[i] == '%':
			for i < len(b) && b[i] != '\n' && b[i] != '\r' {
				i++
			}
		default:
			return i
		}
	}
	return i
}

func isPDFSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0
}

func isDigits(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// pdfText is s as a PDF text string: a literal one for printable ASCII,
// else UTF-16 with its byte order mark, in hex.
func pdfText(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			ascii = false
			break
		}
	}
	if ascii {
		return "(" + strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(s) + ")"
	}
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", u)
	}
	b.WriteString(">")
	return b.String()
}
