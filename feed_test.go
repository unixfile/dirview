package dirview

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// rssFeed is the part of an RSS 2.0 feed the tests read. The channel's
// link is left out: encoding/xml would match atom:link to it too.
type rssFeed struct {
	Version string `xml:"version,attr"`
	Channel struct {
		Title string `xml:"title"`
		Self  struct {
			Href string `xml:"href,attr"`
		} `xml:"http://www.w3.org/2005/Atom link"`
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			GUID        string `xml:"guid"`
			PubDate     string `xml:"pubDate"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

func parseFeed(t *testing.T, rec *httptest.ResponseRecorder) rssFeed {
	t.Helper()
	var f rssFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatalf("feed does not parse: %v\n%s", err, rec.Body.String())
	}
	return f
}

// ?log=rss serves a path's history as RSS: one item per commit, newest
// first, with absolute links to the history and each commit's page.
func TestFeed(t *testing.T) {
	s, dir := repoTree(t)
	h := s.handler()
	rec := get(h, "/a/?log=rss")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d:\n%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/rss+xml; charset=utf-8" {
		t.Errorf("Content-Type %q", ct)
	}
	f := parseFeed(t, rec)
	c := head(t, dir, "HEAD")
	base := "http://127.31.1.1:8080/a/"
	switch {
	case f.Version != "2.0":
		t.Errorf("version %q", f.Version)
	case f.Channel.Title != "git log a/":
		t.Errorf("title %q", f.Channel.Title)
	case !strings.Contains(rec.Body.String(), "<link>"+base+"?log</link>"):
		t.Errorf("no channel link:\n%s", rec.Body.String())
	case f.Channel.Self.Href != base+"?log=rss":
		t.Errorf("self %q", f.Channel.Self.Href)
	case len(f.Channel.Items) != 2:
		t.Fatalf("%d items, want 2:\n%s", len(f.Channel.Items), rec.Body.String())
	}
	it := f.Channel.Items[0]
	if it.Title != "Rename the package <b>" || it.Link != base+"?commit="+c || it.GUID != c {
		t.Errorf("newest item %+v", it)
	}
	if _, err := time.Parse(time.RFC1123Z, it.PubDate); err != nil {
		t.Errorf("pubDate %q: %v", it.PubDate, err)
	}
	if d := f.Channel.Items[1].Description; !strings.Contains(d, "<p>The first line wraps here.<br>- a list item<br>- another</p><p>A second paragraph.</p>") {
		t.Errorf("description not the body in paragraphs: %q", d)
	}
	// A file's feed holds only the commits that touched it.
	if f := parseFeed(t, get(h, "/index.md?log=rss")); len(f.Channel.Items) != 1 || f.Channel.Items[0].Title != "Add the docs" {
		t.Errorf("file feed %+v", f.Channel.Items)
	}
}

// The history page names its feed in its head, for a reader to find, and
// in its header, for the user to see.
func TestHistoryNamesFeed(t *testing.T) {
	s, _ := repoTree(t)
	body := get(s.handler(), "/?log").Body.String()
	for _, want := range []string{
		`<link rel="alternate" type="application/rss+xml" title="git log `,
		`<a href="?log=rss" type="application/rss+xml" title="this history as an RSS feed">rss</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("no %q:\n%s", want, body)
		}
	}
	if body := get(s.handler(), "/").Body.String(); strings.Contains(body, "log=rss") {
		t.Errorf("a listing names the feed:\n%s", body)
	}
}

// A reader that polls with the feed's ETag gets 304 until a commit
// changes the feed.
func TestFeedNotModified(t *testing.T) {
	s, dir := repoTree(t)
	h := s.handler()
	tag := get(h, "/?log=rss").Header().Get("ETag")
	if tag == "" {
		t.Fatal("no ETag")
	}
	if rec := get(h, "/?log=rss", "If-None-Match", tag); rec.Code != http.StatusNotModified {
		t.Errorf("same feed: status %d", rec.Code)
	}
	commitAll(t, dir, "Third")
	if rec := get(h, "/?log=rss", "If-None-Match", tag); rec.Code != http.StatusOK {
		t.Errorf("after a commit: status %d", rec.Code)
	}
}

// A commit message may hold bytes XML forbids; the feed still parses.
func TestFeedForbiddenBytes(t *testing.T) {
	s, dir := repoTree(t)
	commitAll(t, dir, "Bell \x07 in the subject\n\nAnd \x01 in the body & more")
	f := parseFeed(t, get(s.handler(), "/?log=rss"))
	if got := f.Channel.Items[0].Title; got != "Bell \uFFFD in the subject" {
		t.Errorf("title %q", got)
	}
}

// Outside a repo the feed says git's complaint, as the history does.
func TestFeedOutsideRepo(t *testing.T) {
	needGit(t)
	s, _, _ := serveTree(t)
	if rec := get(s.handler(), "/?log=rss"); rec.Code != http.StatusNotFound {
		t.Errorf("status %d", rec.Code)
	}
}

// Links take https from X-Forwarded-Proto only when a proxy on loopback
// sends it.
func TestLinkOrigin(t *testing.T) {
	for _, c := range []struct {
		remote, proto, want string
	}{
		{"127.0.0.1:5000", "https", "https://example.org"},
		{"[::1]:5000", "https", "https://example.org"},
		{"192.0.2.1:5000", "https", "http://example.org"},
		{"127.0.0.1:5000", "gopher", "http://example.org"},
		{"127.0.0.1:5000", "", "http://example.org"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host, r.RemoteAddr = "example.org", c.remote
		if c.proto != "" {
			r.Header.Set("X-Forwarded-Proto", c.proto)
		}
		if got := linkOrigin(r); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.remote, c.proto, got, c.want)
		}
	}
}
