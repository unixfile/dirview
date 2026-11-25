package dirview

import (
	"bytes"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// typstCase writes testdata/typst/NAME.md as typst markup, as the doc
// notes/NAME.md on a server at http://127.31.1.1, where gone.md is
// missing and dot.png is the one image in the tree.
func typstCase(t *testing.T, name string) (src, markup []byte) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", "typst", name+".md"))
	if err != nil {
		t.Fatal(err)
	}
	page, _ := url.Parse("http://127.31.1.1/notes/" + name + ".md")
	missing := func(p string) bool { return p == "gone.md" }
	image := func(dest string) string {
		if dest == "dot.png" {
			return "image1.png"
		}
		return ""
	}
	return src, typstDoc(src, "notes/"+name+".md", pdfPages[0], page, missing, image)
}

// Each doc in testdata/typst writes as the markup beside it, after the
// preamble. Set TYPST_UPDATE=1 to write the markup instead; read the
// diff before committing it.
func TestTypstMarkup(t *testing.T) {
	docs, _ := filepath.Glob(filepath.Join("testdata", "typst", "*.md"))
	if len(docs) == 0 {
		t.Fatal("no docs in testdata/typst")
	}
	for _, doc := range docs {
		name := strings.TrimSuffix(filepath.Base(doc), ".md")
		t.Run(name, func(t *testing.T) {
			_, markup := typstCase(t, name)
			_, body, ok := bytes.Cut(markup, []byte("\n\n"))
			if !ok || !bytes.HasPrefix(markup, []byte(`#set document(title: "notes/`+name+`.md")`)) {
				t.Fatalf("no preamble:\n%s", markup)
			}
			want := filepath.Join("testdata", "typst", name+".typ")
			if os.Getenv("TYPST_UPDATE") != "" {
				os.WriteFile(want, body, 0o644)
				return
			}
			b, err := os.ReadFile(want)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(body, b) {
				t.Errorf("markup differs from %s:\n%s", want, body)
			}
		})
	}
}

// A Graphviz fence draws in the PDF as the SVG dot draws for the page.
// Where dot cannot draw it, the code shows under a line that says why.
func TestTypstGraph(t *testing.T) {
	if _, err := exec.LookPath("dot"); err != nil {
		t.Skip("dot is not installed")
	}
	markup := typstDoc([]byte("```dot\ndigraph { a -> b }\n```\n\n```dot\nnot a graph\n```\n"), "g.md", pdfPages[0], nil, nil, nil)
	if !bytes.Contains(markup, []byte(`#image(bytes("<svg`)) {
		t.Errorf("no SVG for the graph:\n%s", markup)
	}
	if !bytes.Contains(markup, []byte(`#text(font: "Iosevka", size: 0.8em)[Error\:`)) || !bytes.Contains(markup, []byte(`#raw(block: true, "not a graph")`)) {
		t.Errorf("the broken graph is not code under its error:\n%s", markup)
	}
	needTypst(t)
	s, _, dir := serveTree(t)
	os.WriteFile(filepath.Join(dir, "g.md"), []byte("```dot\ndigraph { alpha -> beta }\n```\n"), 0o644)
	words := strings.Fields(pdfTool(t, pdfOf(t, s.handler(), "/g.md"), "pdftotext", "{}", "-"))
	if strings.Join(words, " ") != "alpha beta" {
		t.Errorf("the graph's words: %q", words)
	}
}

// typstString writes any text as a typst string literal.
func TestTypstString(t *testing.T) {
	for in, want := range map[string]string{
		``:                  `""`,
		`plain`:             `"plain"`,
		`a "b" \c`:          `"a \"b\" \\c"`,
		"tab\tline\ncr\r":   `"tab\tline\ncr\r"`,
		"bell\a del\x7f":    `"bell\u{7} del\u{7f}"`,
		"ünï — ☐":           `"ünï — ☐"`,
		`#set text(red)`:    `"#set text(red)"`,
		`\u{41} $x$ <a> @b`: `"\\u{41} $x$ <a> @b"`,
	} {
		if got := typstString(in); got != want {
			t.Errorf("%q: %s, want %s", in, got, want)
		}
	}
}
