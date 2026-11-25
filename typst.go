package dirview

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// typstPreamble sets the look of a saved PDF, as the user's md2pdf-a4
// script sets it: STIX Two Text at 12 pt in justified paragraphs on A4,
// with 47 mm side margins. Code is Iosevka, as on the page, and a code
// block keeps typst's 0.8em, as in md2pdf-a4. Code in a sentence is 0.85em,
// as on the page; typst sets it at 0.8em first, so the rule scales that.
// The preamble holds no blank line, so the doc starts after the first.
// %s is the title as a typst string.
const typstPreamble = `#set document(title: %s)
#set text(font: ("STIX Two Text", "Iosevka"), size: 12pt)
#set page(paper: "a4", margin: (x: 47mm, y: 15mm))
#set par(justify: true)
#show raw: set text(font: "Iosevka")
#show raw.where(block: false): set text(size: 0.85em / 0.8)
`

// typstSource is the file in typst's root that holds the doc's source,
// for the PDF to embed. Images are named image1 and on, so none takes it.
const typstSource = "source"

// typstAttach embeds typstSource in the PDF as the file name, a Markdown
// file, so pdfdetach saves the doc as it was. It is one line, so it can
// lead the preamble.
func typstAttach(name string) string {
	return `#pdf.attach(` + typstString(name) + `, read("` + typstSource + `", encoding: none), mime-type: "text/markdown")` + "\n"
}

// typstOwn sets text of drvu's own, eg an alert's label, in mono at the
// size of code, as the page does.
const typstOwn = `#text(font: "Iosevka", size: 0.8em)[`

// typstBroken marks a link whose target is missing as the page does: a
// dotted underline in the danger colour.
const typstBroken = `#underline(stroke: (paint: rgb("#902000"), thickness: 0.5pt, dash: "dotted"))[`

// typstDoc writes a doc as typst markup from drvu's own goldmark tree, so
// the PDF keeps what the page shows: wiki links, alerts, broken-link marks
// and each block's language. page is the page's URL, against which
// relative links resolve. missing, when given, tells a broken link, as
// renderMarkdown does. image names the file in typst's root that holds a
// doc's image, or "" to show its description instead.
func typstDoc(src []byte, title string, page *url.URL, missing func(string) bool, image func(dest string) string) []byte {
	doc := markdown.Parser().Parse(text.NewReader(src))
	markAlerts(doc, src)
	markLang(doc, src)
	if missing != nil {
		markBroken(doc, missing)
	}
	w := &typstWriter{src: src, page: page, image: image, labels: map[string]bool{}, notes: map[int]*east.Footnote{}, noted: map[int]bool{}}
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Heading:
			if id, ok := n.AttributeString("id"); ok {
				w.labels[string(id.([]byte))] = true
			}
		case *east.Footnote:
			w.notes[n.Index] = n
		}
		return ast.WalkContinue, nil
	})
	w.b.WriteString(strings.Replace(typstPreamble, "%s", typstString(title), 1))
	w.b.WriteByte('\n')
	w.blocks(doc)
	w.b.WriteByte('\n')
	return w.b.Bytes()
}

// typstWriter writes one doc's markup.
type typstWriter struct {
	b     bytes.Buffer
	src   []byte
	page  *url.URL
	image func(string) string
	// labels holds the headings' ids, the targets a link within the doc
	// may name; typst fails on a link to a label that does not exist.
	labels map[string]bool
	// notes holds the footnotes by index; noted, those already written,
	// which a later reference names by label.
	notes map[int]*east.Footnote
	noted map[int]bool
}

// blocks writes parent's block children, apart as paragraphs. A tight
// list item's text runs on into the block after it, as in the page.
func (w *typstWriter) blocks(parent ast.Node) {
	var prev ast.Node
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		if skipBlock(n) {
			continue
		}
		if prev != nil {
			if prev.Kind() == ast.KindTextBlock {
				w.b.WriteByte('\n')
			} else {
				w.b.WriteString("\n\n")
			}
		}
		w.block(n)
		prev = n
	}
}

// skipBlock reports whether a block stays out of the PDF: raw HTML, which
// typst cannot draw, and the footnote list, whose notes stand at their
// references.
func skipBlock(n ast.Node) bool {
	switch n.Kind() {
	case ast.KindHTMLBlock, east.KindFootnoteList:
		return true
	}
	return false
}

// block writes one block, in its own language when it has one.
func (w *typstWriter) block(n ast.Node) {
	if lang, ok := n.AttributeString("lang"); ok {
		w.b.WriteString("#text(lang: " + typstString(string(lang.([]byte))) + ")[")
		defer w.b.WriteByte(']')
	}
	switch n := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		w.inlines(n)
	case *ast.Heading:
		w.b.WriteString("#heading(level: " + strconv.Itoa(n.Level) + ")[")
		w.inlines(n)
		w.b.WriteByte(']')
		if id, ok := n.AttributeString("id"); ok {
			w.b.WriteString("#label(" + typstString(string(id.([]byte))) + ")")
		}
	case *ast.Blockquote:
		w.b.WriteString("#quote(block: true)[")
		w.blocks(n)
		w.b.WriteByte(']')
	case *alertLabel:
		w.b.WriteString(typstOwn)
		w.text(n.kind)
		w.b.WriteByte(']')
	case *ast.List:
		w.list(n)
	case *ast.FencedCodeBlock:
		w.fence(n)
	case *ast.CodeBlock:
		w.code(lines(n, w.src))
	case *ast.ThematicBreak:
		w.b.WriteString("#line(length: 100%)")
	case *east.Table:
		w.table(n)
	case *east.DefinitionList:
		w.terms(n)
	default:
		if n.Type() == ast.TypeBlock && n.FirstChild() != nil && n.FirstChild().Type() == ast.TypeBlock {
			w.blocks(n)
		} else {
			w.inlines(n)
		}
	}
}

// list writes a bullet or numbered list, each item's blocks as one
// argument.
func (w *typstWriter) list(n *ast.List) {
	tight := strconv.FormatBool(n.IsTight)
	if n.IsOrdered() {
		w.b.WriteString("#enum(tight: " + tight + ", start: " + strconv.Itoa(n.Start) + ",\n")
	} else {
		w.b.WriteString("#list(tight: " + tight + ",\n")
	}
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		w.b.WriteByte('[')
		w.blocks(item)
		w.b.WriteString("],\n")
	}
	w.b.WriteByte(')')
}

// fence writes a fenced block: a Graphviz graph as the SVG dot draws, as
// on the page, and anything else as code. A graph dot cannot draw shows as
// code under a line that says why.
func (w *typstWriter) fence(n *ast.FencedCodeBlock) {
	code := lines(n, w.src)
	if lang := n.Language(w.src); lang != nil && isGraph(string(lang)) {
		svg, err := dotSVG(code)
		if err == nil {
			w.b.WriteString("#image(bytes(" + typstString(svg) + "), format: \"svg\")")
			return
		}
		w.b.WriteString(typstOwn)
		w.text(err.Error())
		w.b.WriteString("]\n\n")
	}
	w.code(code)
}

// code writes a code block in the text colour, since code in docs carries
// no syntax colour, and without its final newline, which ends the last
// line.
func (w *typstWriter) code(code string) {
	w.b.WriteString("#raw(block: true, " + typstString(strings.TrimSuffix(code, "\n")) + ")")
}

// lines is a block's text, its lines joined.
func lines(n ast.Node, src []byte) string {
	var b strings.Builder
	ls := n.Lines()
	for i := 0; i < ls.Len(); i++ {
		seg := ls.At(i)
		b.Write(seg.Value(src))
	}
	return b.String()
}

// table writes a table centred, as md2pdf-a4's tables stand, with its
// header row repeated on each page it spans. A row with fewer cells than
// the header gets empty ones; cells past the header's are dropped, as
// GitHub drops them.
func (w *typstWriter) table(n *east.Table) {
	cols := len(n.Alignments)
	w.b.WriteString("#align(center, table(columns: " + strconv.Itoa(cols) + ", align: (")
	// Each name ends in a comma, so one column is still an array.
	for i, a := range n.Alignments {
		if i > 0 {
			w.b.WriteByte(' ')
		}
		switch a {
		case east.AlignLeft:
			w.b.WriteString("left,")
		case east.AlignRight:
			w.b.WriteString("right,")
		case east.AlignCenter:
			w.b.WriteString("center,")
		default:
			w.b.WriteString("auto,")
		}
	}
	w.b.WriteString("),\n")
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		header := row.Kind() == east.KindTableHeader
		if header {
			w.b.WriteString("table.header(")
		}
		cell := row.FirstChild()
		for i := 0; i < cols; i++ {
			if i > 0 {
				w.b.WriteByte(' ')
			}
			w.b.WriteByte('[')
			if cell != nil {
				w.inlines(cell)
				cell = cell.NextSibling()
			}
			w.b.WriteString("],")
		}
		if header {
			w.b.WriteString("),")
		}
		w.b.WriteByte('\n')
	}
	w.b.WriteString("))")
}

// terms writes a definition list. A term without a description of its own,
// because another term follows, gets an empty one; several descriptions
// of one term stand as its paragraphs.
func (w *typstWriter) terms(n *east.DefinitionList) {
	tight := true
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if d, ok := c.(*east.DefinitionDescription); ok && !d.IsTight {
			tight = false
		}
	}
	w.b.WriteString("#terms(tight: " + strconv.FormatBool(tight) + ",\n")
	for c := n.FirstChild(); c != nil; {
		if c.Kind() != east.KindDefinitionTerm {
			c = c.NextSibling()
			continue
		}
		w.b.WriteString("terms.item[")
		w.inlines(c)
		w.b.WriteString("][")
		c = c.NextSibling()
		for first := true; c != nil && c.Kind() == east.KindDefinitionDescription; c = c.NextSibling() {
			if !first {
				w.b.WriteString("\n\n")
			}
			w.blocks(c)
			first = false
		}
		w.b.WriteString("],\n")
	}
	w.b.WriteByte(')')
}

// inlines writes parent's inline children.
func (w *typstWriter) inlines(parent ast.Node) {
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		w.inline(n)
	}
}

func (w *typstWriter) inline(n ast.Node) {
	switch n := n.(type) {
	case *ast.Text:
		v := n.Segment.Value(w.src)
		if n.IsRaw() {
			w.text(string(v))
		} else {
			w.text(markdownText(v))
		}
		switch {
		case n.HardLineBreak():
			w.b.WriteString("\\\n")
		case n.SoftLineBreak():
			w.b.WriteByte('\n')
		}
	case *ast.String:
		if n.IsCode() || !n.IsRaw() {
			w.text(markdownText(n.Value))
		} else {
			w.text(string(n.Value))
		}
	case *ast.CodeSpan:
		var code strings.Builder
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				v := t.Segment.Value(w.src)
				if bytes.HasSuffix(v, []byte("\n")) {
					v = append(v[:len(v)-1:len(v)-1], ' ')
				}
				code.Write(v)
			}
		}
		w.b.WriteString("#raw(" + typstString(code.String()) + ")")
	case *ast.Emphasis:
		if n.Level >= 2 {
			w.b.WriteString("#strong[")
		} else {
			w.b.WriteString("#emph[")
		}
		w.inlines(n)
		w.b.WriteByte(']')
	case *east.Strikethrough:
		w.b.WriteString("#strike[")
		w.inlines(n)
		w.b.WriteByte(']')
	case *ast.Link:
		w.link(n)
	case *ast.AutoLink:
		label, dest := string(n.Label(w.src)), string(n.URL(w.src))
		if n.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(dest), "mailto:") {
			dest = "mailto:" + dest
		}
		if target, ok := w.target(dest); ok {
			w.b.WriteString("#link(" + target + ")[")
			w.text(label)
			w.b.WriteByte(']')
		} else {
			w.text(label)
		}
	case *ast.Image:
		w.picture(n)
	case *ast.RawHTML:
		// Tags stay out, as typst cannot draw them; the text between
		// them is the doc's own and stays. A line break keeps its place.
		if isBreak(n.Segments, w.src) {
			w.b.WriteString("\\\n")
		}
	case *east.TaskCheckBox:
		if n.IsChecked {
			w.text("☑ ")
		} else {
			w.text("☐ ")
		}
	case *east.FootnoteLink:
		w.footnote(n.Index)
	case *east.FootnoteBacklink:
	default:
		w.inlines(n)
	}
}

// link writes a link with its words. A link whose target is missing keeps
// a dotted underline. A link to a heading in the doc leads there; to a
// fragment the doc lacks, or a scheme a PDF should not open, it is words
// alone.
func (w *typstWriter) link(n *ast.Link) {
	target, ok := w.target(string(n.Destination))
	if ok {
		w.b.WriteString("#link(" + target + ")[")
	}
	if class, _ := n.AttributeString("class"); string(asBytes(class)) == "broken" {
		w.b.WriteString(typstBroken)
		w.inlines(n)
		w.b.WriteByte(']')
	} else {
		w.inlines(n)
	}
	if ok {
		w.b.WriteByte(']')
	}
}

func asBytes(v any) []byte {
	b, _ := v.([]byte)
	return b
}

// target is what a link to dest leads to, as typst code: a label in the
// doc, or a URL resolved against the page's. It is not ok for a fragment
// no heading has, or for a scheme other than http, https and mailto.
func (w *typstWriter) target(dest string) (string, bool) {
	u, err := url.Parse(dest)
	if err != nil || dest == "" {
		return "", false
	}
	if u.Scheme == "" && u.Host == "" && u.Path == "" && u.RawQuery == "" && u.Fragment != "" {
		if w.labels[u.Fragment] {
			return "label(" + typstString(u.Fragment) + ")", true
		}
		return "", false
	}
	if w.page != nil {
		u = w.page.ResolveReference(u)
	}
	switch u.Scheme {
	case "http", "https", "mailto":
		return typstString(u.String()), true
	}
	return "", false
}

// picture writes an image from the tree, scaled down to the line's width
// when wider, or its description when it is not from the tree.
func (w *typstWriter) picture(n *ast.Image) {
	alt := plainText(n, w.src)
	file := ""
	if w.image != nil {
		file = w.image(string(n.Destination))
	}
	if file == "" {
		w.text(alt)
		return
	}
	w.b.WriteString("#box(image(" + typstString(file) + ", alt: " + typstString(alt) + "))")
}

// footnote writes a note at its reference. A second reference to the same
// note names the first by its label, which no heading's id can take, since
// an id holds no colon.
func (w *typstWriter) footnote(index int) {
	label := typstString("fn:" + strconv.Itoa(index))
	if w.noted[index] {
		w.b.WriteString("#footnote(label(" + label + "))")
		return
	}
	note := w.notes[index]
	if note == nil {
		return
	}
	w.noted[index] = true
	w.b.WriteString("#footnote[")
	w.blocks(note)
	w.b.WriteString("]#label(" + label + ")")
}

// isBreak reports whether raw inline HTML is a line break: <br>, <br/> or
// <br />, in any case.
func isBreak(segs *text.Segments, src []byte) bool {
	var b []byte
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		b = append(b, seg.Value(src)...)
	}
	tag := strings.ToLower(strings.Join(strings.Fields(string(b)), ""))
	return tag == "<br>" || tag == "<br/>"
}

// plainText is the text below n, as an image's description reads in the
// page.
func plainText(n ast.Node, src []byte) string {
	var b strings.Builder
	ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch c := c.(type) {
		case *ast.Text:
			b.WriteString(markdownText(c.Segment.Value(src)))
			if c.SoftLineBreak() || c.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.WriteString(markdownText(c.Value))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// text writes s as typst markup that reads as s: every ASCII mark that
// could start markup is escaped, which also keeps typst from turning
// quotes and dashes, which the typographer already set, or a URL in
// text, which goldmark already linked where it should. A line break in s,
// eg in dot's message, is a space, as in the page, so two cannot end a
// paragraph.
func (w *typstWriter) text(s string) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\n' || c == '\r':
			c = ' '
		case util.IsPunct(c):
			w.b.WriteByte('\\')
		}
		w.b.WriteByte(c)
	}
}

// markdownText resolves a Markdown text's backslash escapes and entity
// and character references, as goldmark's HTML writer does: \* is *,
// &amp; is & and &#x41; is A. A NUL or a reference to no character is
// U+FFFD.
func markdownText(v []byte) string {
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '\\' && i+1 < len(v) && util.IsPunct(v[i+1]):
			b.WriteByte(v[i+1])
			i++
			continue
		case c == 0:
			b.WriteRune(utf8.RuneError)
			continue
		case c == '&':
			if r, n := reference(v[i:]); n > 0 {
				b.WriteString(r)
				i += n - 1
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// reference reads the entity or character reference at the start of v,
// eg &amp;, &#38; or &#x26;, and gives its text and length, or a length
// of 0 when v starts with none.
func reference(v []byte) (string, int) {
	end := bytes.IndexByte(v, ';')
	if end < 2 || end > 40 {
		return "", 0
	}
	name := v[1:end]
	if name[0] != '#' {
		for _, c := range name {
			if !util.IsAlphaNumeric(c) {
				return "", 0
			}
		}
		if e, ok := util.LookUpHTML5EntityByName(string(name)); ok {
			return string(e.Characters), end + 1
		}
		return "", 0
	}
	digits, base, most := name[1:], 10, 7
	if len(digits) > 0 && (digits[0] == 'x' || digits[0] == 'X') {
		digits, base, most = digits[1:], 16, 6
	}
	if len(digits) == 0 || len(digits) > most {
		return "", 0
	}
	r, err := strconv.ParseUint(string(digits), base, 32)
	if err != nil {
		return "", 0
	}
	if r == 0 || r > utf8.MaxRune || r >= 0xD800 && r <= 0xDFFF {
		return string(utf8.RuneError), end + 1
	}
	return string(rune(r)), end + 1
}

// typstString is s as a typst string literal.
func typstString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			b.WriteString(`\u{` + strconv.FormatInt(int64(r), 16) + `}`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
