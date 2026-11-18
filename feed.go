package www

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// feedLen caps the commits a feed carries. A reader shows what is new
// since its last poll, so the newest few suffice, and the history page
// lists up to maxLog.
const feedLen = 50

// feedQuery is the query that asks for a history as a feed.
const feedQuery = "log=rss"

// isFeed reports whether r asks for a history as a feed.
func isFeed(r *http.Request) bool {
	log, ok := r.URL.Query()["log"]
	return ok && log[0] == "rss"
}

// serveFeed answers name's history as an RSS 2.0 feed: one item per
// commit, newest first, its subject as title and its commit page as link.
// The feed is the history in a second form, so it reads the same commits
// and a share drops the same trailers. An ETag lets a reader that polls
// get 304 until a commit changes the feed.
func (t *tree) serveFeed(w http.ResponseWriter, r *http.Request, name string, dir bool) {
	log, err := t.gitLog(r.Context(), name, feedLen)
	if err != nil {
		crumbs := append(t.pageCrumbs(name, dir), crumb{Label: logLabel})
		t.render(w, r, http.StatusNotFound, page{Title: "git log " + t.title(name), Crumbs: crumbs, Kind: "error",
			Body: template.HTML("<pre class=\"error\">" + template.HTMLEscapeString(err.Error()) + "</pre>")})
		return
	}
	stars := t.starsFor(r.Context(), name)
	// A feed reader keeps links away from the page, so each is absolute.
	here := linkOrigin(r) + r.URL.EscapedPath()
	shown := t.title(name)
	if dir && name != "." {
		shown += "/"
	}
	var b bytes.Buffer
	text := func(s string) {
		// EscapeText also replaces bytes XML forbids, eg a control
		// character in a commit message, so one message cannot break
		// the whole feed.
		xml.EscapeText(&b, []byte(s))
	}
	b.WriteString(xml.Header)
	b.WriteString("<rss version=\"2.0\" xmlns:atom=\"http://www.w3.org/2005/Atom\">\n<channel>\n<title>")
	text("git log " + shown)
	b.WriteString("</title>\n<link>")
	text(here + "?log")
	b.WriteString("</link>\n<description>")
	text("The commits that touched " + shown + ", newest first.")
	b.WriteString("</description>\n<atom:link rel=\"self\" type=\"application/rss+xml\" href=\"")
	text(here + "?" + feedQuery)
	b.WriteString("\"/>\n<generator>")
	text(progName)
	b.WriteString("</generator>\n")
	if len(log) > 0 {
		b.WriteString("<lastBuildDate>" + log[0].date.Format(time.RFC1123Z) + "</lastBuildDate>\n")
	}
	for i := range log {
		c := &log[i]
		b.WriteString("<item>\n<title>")
		text(subjectOf(c))
		b.WriteString("</title>\n<link>")
		text(here + "?commit=" + c.hash)
		b.WriteString("</link>\n<guid isPermaLink=\"false\">" + c.hash + "</guid>\n")
		b.WriteString("<pubDate>" + c.date.Format(time.RFC1123Z) + "</pubDate>\n<description>")
		// The description is HTML, escaped once more as XML text: the
		// stardate and hash as the history shows them, then the body
		// in paragraphs.
		var d strings.Builder
		fmt.Fprintf(&d, "<p>%s <code>%s</code></p>", template.HTMLEscapeString(stars.label(c)), c.short)
		for _, para := range paragraphs(c.body) {
			d.WriteString("<p>" + para + "</p>")
		}
		text(d.String())
		b.WriteString("</description>\n</item>\n")
	}
	b.WriteString("</channel>\n</rss>\n")
	sum := sha256.Sum256(b.Bytes())
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:12])+`"`)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(b.Bytes()))
}

// linkOrigin is the scheme and host r reached, eg http://127.31.1.1, for links
// that leave the page. A proxy on loopback that ends TLS, such as Caddy
// before www-public, names the scheme in X-Forwarded-Proto; any other
// peer's header is ignored, since it could claim anything.
func linkOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p == "https" || p == "http" {
		if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil && ap.Addr().Unmap().IsLoopback() {
			scheme = p
		}
	}
	return scheme + "://" + r.Host
}
