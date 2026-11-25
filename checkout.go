package dirview

import (
	"bufio"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// A cached repo's page links to the user's checkout of the same repo, at
// the same file: a git repo in or below a tree the default address serves
// whose origin names the repo. The web address finds checkouts in those
// trees alone, so it never scans home, and a public server, which has no
// default address, shows no link.

// checkoutDepth is how far below a tree's root a checkout is looked for,
// so a tree at ~/scratch finds ~/scratch/github.com/owner/repo.
const checkoutDepth = 3

// checkoutFresh is how long the checkouts found are kept before a page
// sets a scan going again, so a clone made meanwhile gets its link.
const checkoutFresh = time.Minute

// checkout is a repo's checkout in a served tree.
type checkout struct {
	tree *tree  // the tree it lies in
	dir  string // its directory
	rel  string // its path in the tree, slashed, "" at the tree's root
}

// checkouts finds the user's checkouts in the trees of home, the default
// address's server, by the repo key of their origin.
//
// A scan runs beside the pages, never in one: a tree on a slow disk took
// 9 s to scan cold on the user's machine on 2026-11-25, where a page takes
// milliseconds. A page reads what the last scan found, so the first page
// after a tree joins may lack its link.
type checkouts struct {
	home *server
	// origin is the default address's URL without basePath, which every
	// tree's prefix carries.
	origin string

	mu       sync.Mutex
	at       time.Time
	trees    []*tree             // the trees the last scan read
	byKey    map[string]checkout // by repoKey's fold of the key
	scanning bool
	wg       sync.WaitGroup // the scan under way, which a test waits for
}

// find returns the checkout of the repo key, if a served tree holds one,
// as the last scan found it. It sets a scan going when none has run, the
// served trees changed or checkoutFresh has passed. A checkout in a tree
// no longer served is not returned, so no link leads to a tree gone.
func (c *checkouts) find(key string) (checkout, bool) {
	if c == nil || c.home == nil {
		return checkout{}, false
	}
	trees := c.home.list()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.scanning && (c.byKey == nil || time.Since(c.at) > checkoutFresh || !slices.Equal(trees, c.trees)) {
		c.scanning = true
		c.wg.Add(1)
		go c.scan(trees)
	}
	co, ok := c.byKey[foldKey(key)]
	if !ok || !slices.Contains(trees, co.tree) {
		return checkout{}, false
	}
	return co, true
}

// scan reads the checkouts in trees and keeps what it found.
func (c *checkouts) scan(trees []*tree) {
	defer c.wg.Done()
	found := scanCheckouts(trees)
	c.mu.Lock()
	c.byKey, c.at, c.trees, c.scanning = found, time.Now(), trees, false
	c.mu.Unlock()
}

// scanCheckouts reads the origin of each repo in or below trees, at most
// checkoutDepth below a root. Where two checkouts name one repo, the
// shorter path wins, then the first by name, so the same one wins each
// time; a tree inside another serves its checkout under its own prefix.
func scanCheckouts(trees []*tree) map[string]checkout {
	out := map[string]checkout{}
	for _, t := range trees {
		if t.shared || t.secret || t.at != nil || t.web != nil {
			continue
		}
		findRepos(t.realDir, checkoutDepth, func(dir string) {
			key := repoKey(originURL(dir))
			if key == "" {
				return
			}
			rel, err := filepath.Rel(t.realDir, dir)
			if err != nil {
				return
			}
			rel = filepath.ToSlash(rel)
			if rel == "." {
				rel = ""
			}
			co := checkout{tree: t, dir: dir, rel: rel}
			if old, ok := out[key]; ok && !closer(co, old) {
				return
			}
			out[key] = co
		})
	}
	return out
}

// closer reports whether a lies nearer its tree's root than b, or as near
// and first by path.
func closer(a, b checkout) bool {
	da, db := strings.Count(a.rel, "/")+min(len(a.rel), 1), strings.Count(b.rel, "/")+min(len(b.rel), 1)
	if da != db {
		return da < db
	}
	return a.dir < b.dir
}

// findRepos calls found for each directory at or below dir, at most depth
// levels down, that holds .git, and goes no deeper inside one. It skips
// dot directories and follows no links, so it stays inside the tree.
func findRepos(dir string, depth int, found func(string)) {
	if exists(filepath.Join(dir, ".git")) {
		found(dir)
		return
	}
	if depth == 0 {
		return
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			findRepos(filepath.Join(dir, e.Name()), depth-1, found)
		}
	}
}

// originURL is the URL of the remote origin in the config of the repo
// whose work tree is dir, or "". It reads the file, where git would cost
// a process per repo on each scan; a URL set by an include or rewritten by
// insteadOf goes unseen.
func originURL(dir string) string {
	gitDir := filepath.Join(dir, ".git")
	if b, err := os.ReadFile(gitDir); err == nil {
		// A worktree's or submodule's .git is a file naming its git
		// directory, whose commondir names the repo's.
		p, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
		if !ok {
			return ""
		}
		gitDir = strings.TrimSpace(p)
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(dir, gitDir)
		}
		if c, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
			common := strings.TrimSpace(string(c))
			if !filepath.IsAbs(common) {
				common = filepath.Join(gitDir, common)
			}
			gitDir = common
		}
	}
	f, err := os.Open(filepath.Join(gitDir, "config"))
	if err != nil {
		return ""
	}
	defer f.Close()
	return configValue(bufio.NewScanner(f), `remote "origin"`, "url")
}

// configValue is the last value of name in section of a git config, as
// git takes the last, or "". It knows the common form: a header such as
// [remote "origin"], then name = value lines.
func configValue(sc *bufio.Scanner, section, name string) string {
	in, val := false, ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			head, _, _ := strings.Cut(line[1:], "]")
			// git compares the section's name without case, the
			// subsection's with it.
			sec, sub, quoted := strings.Cut(strings.TrimSpace(head), " ")
			want, wantSub, _ := strings.Cut(section, " ")
			in = strings.EqualFold(sec, want) && (quoted && strings.TrimSpace(sub) == wantSub)
			continue
		}
		if !in {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(k), name) {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		val = v
	}
	return val
}

// repoKey is the key a cached repo goes by for a remote URL, eg
// github.com/junegunn/fzf for https://github.com/junegunn/fzf.git or
// git@github.com:junegunn/fzf, folded as foldKey folds, or "" for a URL
// that names no repo on a host, eg a path on this machine.
func repoKey(remote string) string {
	var host, p string
	switch {
	case remote == "":
		return ""
	case strings.Contains(remote, "://"):
		u, err := url.Parse(remote)
		if err != nil || u.Host == "" {
			return ""
		}
		host, p = u.Host, u.Path
		switch u.Scheme {
		case "https", "http", "git":
			// A default port names the same host; another port, eg a
			// test forge's, is part of the key, as on the web address.
			if h, port, err := net.SplitHostPort(host); err == nil && (port == "443" && u.Scheme == "https" || port == "80" && u.Scheme == "http" || port == "9418" && u.Scheme == "git") {
				host = h
			}
		case "ssh", "git+ssh":
			// An ssh port says how to log in, not which repo.
			host = u.Hostname()
		default:
			return ""
		}
	default:
		// scp's form, [user@]host:path, where the colon comes before
		// any slash.
		at := strings.LastIndex(remote, "@")
		colon := strings.Index(remote, ":")
		if colon < 0 || strings.Contains(remote[:colon], "/") {
			return ""
		}
		host, p = remote[at+1:colon], remote[colon+1:]
		if strings.Contains(host, "@") || host == "" {
			return ""
		}
	}
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	p = strings.TrimSuffix(strings.Trim(path.Clean("/"+p), "/"), ".git")
	if host == "" || p == "" || p == "." {
		return ""
	}
	return foldKey(host + "/" + p)
}

// foldKey folds a repo's key for comparison: the forges in depths and
// minDepths ignore case in a repo's path, so their keys fold whole; other
// hosts keep the path's case.
func foldKey(key string) string {
	host, rest, _ := strings.Cut(key, "/")
	host = strings.ToLower(host)
	if _, ok := depths[host]; ok {
		rest = strings.ToLower(rest)
	} else if _, ok := minDepths[host]; ok {
		rest = strings.ToLower(rest)
	}
	return host + "/" + rest
}

// link is the URL of name in the checkout, on the default address: the
// same file or directory where the checkout has it, else the checkout's
// root, since a file the cached commit holds may be gone or renamed in
// the checkout.
func (c *checkouts) link(co checkout, name string) string {
	p := co.tree.prefix + co.rel
	if co.rel != "" {
		p += "/"
	}
	if name != "." {
		if info, err := os.Stat(filepath.Join(co.dir, filepath.FromSlash(name))); err == nil {
			p += name
			if info.IsDir() {
				p += "/"
			}
		}
	}
	return c.origin + (&url.URL{Path: p}).EscapedPath()
}

// checkoutLink is the page r asks of t, a repo on the web address or a
// version of one, in the user's checkout of that repo, and the
// checkout's directory as a page shows it, or "" for both.
func (t *tree) checkoutLink(r *http.Request) (link, dir string) {
	src := t
	if t.base != nil {
		src = t.base
	}
	if src.web == nil || src.srv == nil || src.srv.web == nil {
		return "", ""
	}
	c := src.srv.web.checkouts
	co, ok := c.find(src.web.key)
	if !ok {
		return "", ""
	}
	return c.link(co, t.name(r.URL.Path)), tildePath(co.dir)
}
