package www

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeSyntax puts a helper on PATH that prints a fixed tree for .go files
// and refuses the rest, as the real one does without a grammar.
func fakeSyntax(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in *.go) ;; *) echo \"E95: no grammar for $1\" >&2; exit 95;; esac\ncat >/dev/null\n" +
		"printf '0\\tsource_file\\t\\t0\\t0\\t3\\t0\\n1\\tfunction_declaration\\t\\t0\\t0\\t2\\t1\\n2\\tidentifier\\tname\\t0\\t5\\t0\\t6\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, progName+"-syntax"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// A tree page folds each node with children, opens the root, and gives
// each row its field, kind, text and lines.
func TestSyntaxPage(t *testing.T) {
	fakeSyntax(t)
	src := []byte("func f() {\n\treturn\n}\n")
	tree, err := syntaxPage(context.Background(), "a.go", pageHref("/t/a.go"), src)
	if err != nil {
		t.Fatal(err)
	}
	h := tree.html
	for _, want := range []string{
		`<details open><summary><b>source_file</b> <code>func f() { …</code> <a href="a.go#L1-L3">1–3</a></summary>`,
		`<details><summary><b>function_declaration</b> <code>func f() { …</code> <a href="a.go#L1-L3">1–3</a></summary>`,
		`<div><i>name:</i> <b>identifier</b> <code>f</code> <a href="a.go#L1">1</a></div>`,
	} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %s in\n%s", want, h)
		}
	}
	if strings.Count(h, "<details") != strings.Count(h, "</details>") {
		t.Errorf("folds not closed:\n%s", h)
	}
}

// A tree page sends only what shows: the open root, and each closed fold
// with its summary and no rows. A fold's rows come on request, with the
// folds inside it empty in turn, and together they make the whole tree.
func TestSyntaxFoldsComeWhenOpened(t *testing.T) {
	src := []byte("a\nb\nc\nd\n")
	w := newSynRows("x.go", src)
	for _, n := range []synNode{
		{depth: 0, kind: "root", sr: 0, er: 4},
		{depth: 1, kind: "f", sr: 0, er: 2},
		{depth: 2, kind: "g", sr: 0, er: 1},
		{depth: 3, kind: "leaf1", sr: 0, er: 0, ec: 1},
		{depth: 2, kind: "leaf2", sr: 1, er: 1, ec: 1},
		{depth: 1, kind: "h", sr: 2, er: 3},
		{depth: 2, kind: "leaf3", sr: 2, er: 2, ec: 1},
		{depth: 1, kind: "leaf4", sr: 3, er: 3, ec: 1},
	} {
		w.add(n)
	}
	tree := &synTree{html: w.done(), folds: w.folds, sum: "s"}
	if len(tree.folds) != 3 {
		t.Fatalf("%d closed folds, want 3: %+v", len(tree.folds), tree.folds)
	}
	page := tree.page()
	for _, want := range []string{`<div class="syntax" data-sum="s">`, `<details open><summary><b>root</b>`,
		`<details data-fold="0"><summary><b>f</b>`, `<details data-fold="2"><summary><b>h</b>`, `<b>leaf4</b>`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %s:\n%s", want, page)
		}
	}
	for _, gone := range []string{"<b>g</b>", "leaf1", "leaf2", "leaf3"} {
		if strings.Contains(page, gone) {
			t.Errorf("page carries %s, which sits in a closed fold:\n%s", gone, page)
		}
	}
	f0, _ := tree.fold(0)
	if !strings.Contains(f0, `<details data-fold="1"><summary><b>g</b>`) || !strings.Contains(f0, "leaf2") || strings.Contains(f0, "leaf1") {
		t.Errorf("fold 0:\n%s", f0)
	}
	if _, ok := tree.fold(3); ok {
		t.Error("a fold past the last")
	}
	// Each fold's rows put back in place give the whole tree.
	whole := page
	for strings.Contains(whole, "data-fold=") {
		i := strings.Index(whole, `<details data-fold="`)
		j := i + len(`<details data-fold="`)
		k := j + strings.IndexByte(whole[j:], '"')
		n, _ := strconv.Atoi(whole[j:k])
		rows, _ := tree.fold(n)
		end := k + strings.Index(whole[k:], "</summary>\n") + len("</summary>\n")
		whole = whole[:i] + "<details>" + whole[k+2:end] + rows + whole[end:]
	}
	if want := strings.Replace(tree.html, `<div class="syntax">`, `<div class="syntax" data-sum="s">`, 1); whole != want {
		t.Errorf("folds put back:\n%s\nwant:\n%s", whole, want)
	}
}

// A fold is asked for by number and by the hash of the source the page
// showed: a changed file gets 409, a fold that does not exist 404, and
// "all" the whole tree.
func TestSyntaxFoldRequests(t *testing.T) {
	fakeSyntax(t)
	s, _, dir := serveTree(t)
	os.WriteFile(filepath.Join(dir, "f.go"), []byte("func f() {\n\treturn\n}\n"), 0o644)
	page := get(s.handler(), "/f.go?tree").Body.String()
	i := strings.Index(page, `data-sum="`)
	if i < 0 || !strings.Contains(page, `<details data-fold="0"><summary><b>function_declaration</b>`) || strings.Contains(page, "identifier") {
		t.Fatalf("tree page:\n%s", page)
	}
	sum := page[i+len(`data-sum="`) : i+len(`data-sum="`)+12]
	for q, want := range map[string]struct {
		code int
		body string
	}{
		"fold=0&sum=" + sum:       {200, "<b>identifier</b>"},
		"fold=all&sum=" + sum:     {200, "<b>function_declaration</b>"},
		"fold=7&sum=" + sum:       {404, "no fold 7"},
		"fold=x&sum=" + sum:       {404, "no fold x"},
		"fold=0&sum=000000000000": {409, "the file changed"},
	} {
		rec := get(s.handler(), "/f.go?tree&"+q)
		if rec.Code != want.code || !strings.Contains(rec.Body.String(), want.body) {
			t.Errorf("%s: %d %s, want %d %s", q, rec.Code, rec.Body, want.code, want.body)
		}
	}
	if rec := get(s.handler(), "/f.go?tree&fold=0&sum="+sum); strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("a fold came as a whole page: %s", rec.Body)
	}
}

// Without the helper, or without a grammar, a tree page says so.
func TestSyntaxMissing(t *testing.T) {
	fakeSyntax(t)
	if _, err := syntaxNodes(context.Background(), "x.txt", nil); !errors.Is(err, errNoGrammar) {
		t.Errorf("no grammar: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := syntaxNodes(context.Background(), "b.go", nil); !errors.Is(err, errNoHelper) {
		t.Errorf("no helper: %v", err)
	}
}

// Output the helper should never print fails the page with what was
// wrong, and so does a helper that dies after its first rows.
func TestSyntaxBadOutput(t *testing.T) {
	for _, tc := range []struct{ script, want string }{
		{`printf '0\tsource_file\t\t0\t0\n'`, "a line of 5 fields"},
		{`printf '0\tsource_file\t\t0\t-1\t3\t0\n'`, "a bad number: -1"},
		{`printf '0\tsource_file\t\t0\t0\t3\t0\n'; echo 'E5: broke' >&2; exit 5`, "E5: broke"},
		{`printf '0\tsource_file\t\t0'; echo 'E5: broke' >&2; exit 5`, "E5: broke"},
	} {
		dir := t.TempDir()
		script := "#!/bin/sh\ncat >/dev/null\n" + tc.script + "\n"
		if err := os.WriteFile(filepath.Join(dir, progName+"-syntax"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		if _, err := syntaxPage(context.Background(), "a.go", "a.go", []byte("x\n")); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want %q", tc.script, err, tc.want)
		}
	}
}

// An excerpt keeps a node's first row, its spaces folded, and marks text
// cut or continued.
func TestExcerpt(t *testing.T) {
	lines := [][]byte{[]byte("x := f(a,\t  b)"), []byte("y"), []byte(strings.Repeat("z", 80))}
	for _, tc := range []struct {
		n    synNode
		want string
	}{
		{synNode{sr: 0, sc: 5, er: 0, ec: 14}, "f(a, b)"},
		{synNode{sr: 0, sc: 0, er: 1, ec: 1}, "x := f(a, b) …"},
		{synNode{sr: 0, sc: 0, er: 1, ec: 0}, "x := f(a, b)"},
		{synNode{sr: 2, sc: 0, er: 2, ec: 80}, strings.Repeat("z", excerptLen) + " …"},
		{synNode{sr: 5}, ""},
	} {
		if got := excerpt(lines, tc.n); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.n, got, tc.want)
		}
	}
}

// writeExcerpt writes what excerpt gives, escaped, wherever a cut lands:
// on a word, on the space that joins two, inside a multibyte character,
// and in rows with escapes, a NUL, Unicode spaces or invalid UTF-8.
func TestWriteExcerpt(t *testing.T) {
	word := strings.Repeat("z", excerptLen-1)
	lines := [][]byte{
		[]byte(word + " a b"),
		[]byte(word + "  \t\u00a0b"),
		[]byte(word + "é" + " é"),
		[]byte(" \t<a href=\"x\">'&'</a>\x00 \u2003end "),
		[]byte(strings.Repeat("ü", excerptLen) + "\u0085"),
		[]byte(strings.Repeat("y", 70) + "\xff z"),
		[]byte("ok \xffbad"),
		[]byte("   "),
	}
	var b strings.Builder
	for sr, line := range lines {
		for sc := range len(line) + 1 {
			for _, n := range []synNode{
				{sr: sr, sc: sc, er: sr, ec: len(line)},
				{sr: sr, sc: sc, er: sr + 1, ec: 1},
				{sr: sr, sc: sc, er: sr + 1, ec: 0},
				{sr: sr, sc: sc, er: sr, ec: sc + 3},
			} {
				var want strings.Builder
				if s := excerpt(lines, n); s != "" {
					want.WriteString(" <code>")
					escapeHTML(&want, s)
					want.WriteString("</code>")
				}
				b.Reset()
				writeExcerpt(&b, lines, n)
				if b.String() != want.String() {
					t.Errorf("%+v: %q, want %q", n, b.String(), want.String())
				}
			}
		}
	}
}
