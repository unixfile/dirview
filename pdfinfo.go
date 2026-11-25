package dirview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
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

// info adds keys to the PDF's document info.
func (u *pdfUpdate) info(keys []pdfKey) error {
	if u.f.trailer.get("/Info") == "" {
		return fmt.Errorf("%w: no document info", errPDFForm)
	}
	num, gen, d, err := u.f.dict(u.f.trailer.get("/Info"))
	if err != nil {
		return err
	}
	for _, k := range keys {
		d.set("/"+k.name, pdfText(k.value))
	}
	u.set(num, gen, d.bytes())
	return nil
}
