package dirview

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
)

// errPDFForm is the error for a PDF that drvu cannot update.
var errPDFForm = errors.New("not a PDF with a classic cross-reference table")

// pdfFile is a PDF that drvu adds to in an incremental update: the bytes
// typst wrote stay as they are, and new versions of the objects that
// change, a cross-reference section and a trailer follow them. It reads
// the classic cross-reference table that typst writes, and fails on any
// other form.
type pdfFile struct {
	b       []byte
	xref    int
	trailer *pdfDictionary
}

func readPDF(b []byte) (*pdfFile, error) {
	at := bytes.LastIndex(b, []byte("startxref"))
	if at < 0 {
		return nil, fmt.Errorf("%w: no startxref", errPDFForm)
	}
	xref, err := strconv.Atoi(pdfToken(b, at+len("startxref")))
	if err != nil || xref < 0 || xref >= len(b) {
		return nil, fmt.Errorf("%w: startxref names no offset", errPDFForm)
	}
	t, err := pdfTrailer(b, xref)
	if err != nil {
		return nil, err
	}
	if t.get("/Root") == "" || t.get("/Size") == "" {
		return nil, fmt.Errorf("%w: the trailer names no root or size", errPDFForm)
	}
	return &pdfFile{b: b, xref: xref, trailer: t}, nil
}

// object is the text of the object ref names, eg "18 0 R", and its
// number and generation.
func (f *pdfFile) object(ref string) (num, gen int, text string, err error) {
	num, gen, ok := pdfRef(ref)
	if !ok {
		return 0, 0, "", fmt.Errorf("%w: %q is no reference", errPDFForm, ref)
	}
	off, err := pdfObject(f.b, f.xref, num, gen)
	if err != nil {
		return 0, 0, "", err
	}
	head := fmt.Sprintf("%d %d obj", num, gen)
	i := pdfSkip(f.b, off)
	if !bytes.HasPrefix(f.b[i:], []byte(head)) {
		return 0, 0, "", fmt.Errorf("%w: object %d is not where the table says", errPDFForm, num)
	}
	start := pdfSkip(f.b, i+len(head))
	end, err := pdfValue(f.b, start)
	if err != nil {
		return 0, 0, "", fmt.Errorf("%w: object %d: %v", errPDFForm, num, err)
	}
	return num, gen, string(f.b[start:end]), nil
}

// dict is the dictionary object ref names.
func (f *pdfFile) dict(ref string) (num, gen int, d *pdfDictionary, err error) {
	num, gen, text, err := f.object(ref)
	if err != nil {
		return 0, 0, nil, err
	}
	d, err = pdfDict([]byte(text), 0)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("%w: object %d: %v", errPDFForm, num, err)
	}
	return num, gen, d, nil
}

// pdfUpdate collects the objects of one incremental update.
type pdfUpdate struct {
	f    *pdfFile
	size int
	objs map[int]pdfObj
}

type pdfObj struct {
	gen  int
	body []byte
}

func (f *pdfFile) update() (*pdfUpdate, error) {
	size, err := strconv.Atoi(f.trailer.get("/Size"))
	if err != nil {
		return nil, fmt.Errorf("%w: the trailer's size", errPDFForm)
	}
	return &pdfUpdate{f: f, size: size, objs: map[int]pdfObj{}}, nil
}

// add makes body a new object and returns its reference.
func (u *pdfUpdate) add(body []byte) string {
	n := u.size
	u.size++
	u.objs[n] = pdfObj{0, body}
	return fmt.Sprintf("%d 0 R", n)
}

// set replaces object num in generation gen with body.
func (u *pdfUpdate) set(num, gen int, body []byte) {
	u.objs[num] = pdfObj{gen, body}
}

// bytes is the PDF with the update appended. The new trailer keeps the
// old one's keys, eg the root, the info and the ID, and points back at
// the old table.
func (u *pdfUpdate) bytes() []byte {
	pdf := u.f.b
	out := bytes.NewBuffer(make([]byte, 0, len(pdf)+4096))
	out.Write(pdf)
	if !bytes.HasSuffix(pdf, []byte("\n")) {
		out.WriteByte('\n')
	}
	nums := make([]int, 0, len(u.objs))
	for n := range u.objs {
		nums = append(nums, n)
	}
	slices.Sort(nums)
	offs := map[int]int{}
	for _, n := range nums {
		offs[n] = out.Len()
		fmt.Fprintf(out, "%d %d obj\n%s\nendobj\n", n, u.objs[n].gen, u.objs[n].body)
	}
	table := out.Len()
	out.WriteString("xref\n")
	// One subsection for each run of consecutive numbers.
	for i := 0; i < len(nums); {
		j := i + 1
		for j < len(nums) && nums[j] == nums[j-1]+1 {
			j++
		}
		fmt.Fprintf(out, "%d %d\n", nums[i], j-i)
		for _, n := range nums[i:j] {
			fmt.Fprintf(out, "%010d %05d n \n", offs[n], u.objs[n].gen)
		}
		i = j
	}
	t := u.f.trailer.clone()
	t.set("/Size", strconv.Itoa(u.size))
	t.set("/Prev", strconv.Itoa(u.f.xref))
	fmt.Fprintf(out, "trailer\n%s\nstartxref\n%d\n%%%%EOF\n", t.bytes(), table)
	return out.Bytes()
}

// pdfDictionary is a dictionary as it stands in a PDF: its keys, names
// with their slash, in order, each with its value's text.
type pdfDictionary struct {
	keys []string
	vals map[string]string
}

func (d *pdfDictionary) get(k string) string { return d.vals[k] }

func (d *pdfDictionary) set(k, v string) {
	if _, ok := d.vals[k]; !ok {
		d.keys = append(d.keys, k)
	}
	d.vals[k] = v
}

func (d *pdfDictionary) clone() *pdfDictionary {
	c := &pdfDictionary{keys: slices.Clone(d.keys), vals: map[string]string{}}
	for k, v := range d.vals {
		c.vals[k] = v
	}
	return c
}

func (d *pdfDictionary) bytes() []byte {
	var b bytes.Buffer
	b.WriteString("<<\n")
	for _, k := range d.keys {
		fmt.Fprintf(&b, "  %s %s\n", k, d.vals[k])
	}
	b.WriteString(">>")
	return b.Bytes()
}

// pdfTrailer reads the trailer of the cross-reference section at xref.
func pdfTrailer(pdf []byte, xref int) (*pdfDictionary, error) {
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
				if len(e) != 3 || e[2] != "n" || e[1] != fmt.Sprintf("%05d", gen) {
					return 0, fmt.Errorf("%w: object %d is not in use", errPDFForm, num)
				}
				off, err := strconv.Atoi(e[0])
				if err != nil || off >= len(pdf) {
					return 0, fmt.Errorf("%w: object %d has no offset", errPDFForm, num)
				}
				return off, nil
			}
			i += 20 * count
		}
		t, err := pdfTrailer(pdf, xref)
		if err != nil {
			return 0, err
		}
		if xref, err = strconv.Atoi(t.get("/Prev")); err != nil {
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

// pdfRefs reads an array of indirect references, eg "[18 0 R 20 0 R]".
func pdfRefs(v string) ([]string, bool) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return nil, false
	}
	f := strings.Fields(v[1 : len(v)-1])
	if len(f)%3 != 0 {
		return nil, false
	}
	var refs []string
	for i := 0; i < len(f); i += 3 {
		ref := strings.Join(f[i:i+3], " ")
		if _, _, ok := pdfRef(ref); !ok {
			return nil, false
		}
		refs = append(refs, ref)
	}
	return refs, true
}

// pdfDict reads the dictionary at i.
func pdfDict(b []byte, i int) (*pdfDictionary, error) {
	if !bytes.HasPrefix(b[i:], []byte("<<")) {
		return nil, errors.New("no dictionary")
	}
	d := &pdfDictionary{vals: map[string]string{}}
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
		d.set(key, string(b[start:end]))
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
		return pdfLiteral(s)
	}
	var b strings.Builder
	b.WriteString("<FEFF")
	for _, u := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", u)
	}
	b.WriteString(">")
	return b.String()
}

// pdfLiteral is s, bytes as they are, as a PDF literal string.
func pdfLiteral(s string) string {
	return "(" + strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`).Replace(s) + ")"
}
