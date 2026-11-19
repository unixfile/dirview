package dirview

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// privateHost is the forge whose private repos the reader's server reads
// with gh's login. gh logs in to GitHub. Tests point it at their forge.
var privateHost = "github.com"

// privateAuth returns the token git sends to privateHost for a repo in
// the private cache. It stays nil in the public executable, so the
// linker leaves ghToken out of it; the reader's server sets it when it
// serves the web address.
var privateAuth func() (string, error)

// ghTokenFresh is how long a token from gh is used before gh is asked
// again. A page of a private repo may fetch a blob, and asking gh takes
// some 50 ms.
const ghTokenFresh = 5 * time.Minute

var ghCache struct {
	sync.Mutex
	token string
	at    time.Time
}

// ghToken asks gh for its login to github.com, and keeps the answer for
// ghTokenFresh. Its error names what is missing.
func ghToken() (string, error) {
	ghCache.Lock()
	defer ghCache.Unlock()
	if ghCache.token != "" && time.Since(ghCache.at) < ghTokenFresh {
		return ghCache.token, nil
	}
	gh, err := exec.LookPath("gh")
	if err != nil {
		return "", &errno{syscall.ENOENT, progName + " reads private repos through gh, which is not installed"}
	}
	out, err := exec.Command(gh, "auth", "token", "--hostname", "github.com").Output()
	token := strings.TrimSpace(string(out))
	if err != nil || token == "" {
		why := "gh is not logged in to github.com; gh auth login logs it in"
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			why += ": " + strings.TrimSpace(string(ee.Stderr))
		}
		return "", &errno{syscall.EACCES, why}
	}
	ghCache.token, ghCache.at = token, time.Now()
	return token, nil
}

// privateEnv is the environment that gives git the login for privateHost,
// through GIT_CONFIG_* rather than an argument, which every process on
// the machine can read in /proc. The header goes to privateHost alone,
// so a redirect elsewhere never carries it. Without a login git asks for
// one and fails, as it does for any repo outside the private cache.
func privateEnv() []string {
	if privateAuth == nil {
		return nil
	}
	token, err := privateAuth()
	if err != nil {
		return nil
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return []string{"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http." + webScheme + "://" + privateHost + "/.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic " + basic}
}

// usePrivate lets w read private repos into the cache beside root, eg
// ~/.cache/drvu-private beside ~/.cache/drvu. A public server shares root
// when it runs as the same user, but never opens this one.
func (w *web) usePrivate(root string) error {
	dir := filepath.Join(filepath.Dir(root), progName+"-private")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	w.private, privateRoot, privateAuth = real, real, ghToken
	return nil
}
