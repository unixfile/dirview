package dirview

import (
	"go/scanner"
	"go/token"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

// The program's name lives in NAME alone and the project's in go.mod. Go
// code takes the first from progName; the Makefile reads NAME and takes the
// project's from the module, and fills both into the package files. So a
// tracked file names either only in prose: comments, README.md and doc/,
// or where Go needs a literal: go.mod, the package clause and the import
// in each main under cmd/.
func TestNameHasOneSource(t *testing.T) {
	needGit(t)
	out, err := exec.Command("git", "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	mod, err := exec.Command("go", "list", "-m").Output()
	if err != nil {
		t.Fatalf("go list -m: %v", err)
	}
	names := []string{strings.ToLower(progName), strings.ToLower(path.Base(strings.TrimSpace(string(mod))))}
	holds := func(s string) bool {
		s = strings.ToLower(s)
		for _, n := range names {
			if strings.Contains(s, n) {
				return true
			}
		}
		return false
	}
	for _, f := range strings.Fields(string(out)) {
		if f == "NAME" || f == "go.mod" || f == "README.md" || strings.HasPrefix(f, "doc/") || strings.HasPrefix(f, "assets/fonts/") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if path.Ext(f) == ".go" {
			var s scanner.Scanner
			fset := token.NewFileSet()
			s.Init(fset.AddFile(f, -1, len(src)), src, nil, 0)
			imports := false
			for {
				pos, tok, lit := s.Scan()
				if tok == token.EOF {
					break
				}
				// An import clause ends at the first declaration after it.
				switch tok {
				case token.IMPORT:
					imports = true
				case token.FUNC, token.VAR, token.CONST, token.TYPE:
					imports = false
				}
				if imports {
					continue
				}
				if (tok == token.STRING || tok == token.CHAR) && holds(lit) {
					t.Errorf("%s: %s", fset.Position(pos), lit)
				}
			}
			continue
		}
		for i, line := range strings.Split(string(src), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "#") && holds(line) {
				t.Errorf("%s:%d: %s", f, i+1, line)
			}
		}
	}
}
