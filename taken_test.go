package dirview

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stray makes a drvu of this user that holds ep with no live tree: a
// sleeping child stands in for the server, its record names the child,
// and the test listens on ep in its place.
func stray(t *testing.T) (ep netip.AddrPort, pid string, done chan error) {
	t.Helper()
	ep = testEndpoint(t)
	ln, err := net.Listen("tcp", ep.String())
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { ln.Close() })
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip(err)
	}
	done = make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { cmd.Process.Kill() })
	reg, err := openRegistry(ep)
	if err != nil {
		t.Fatal(err)
	}
	reg.close()
	rec := fmt.Sprintf("pid: %d\nboot_id: %s\npid_ns: %s\n", cmd.Process.Pid, bootID(), pidNS())
	if err := os.WriteFile(filepath.Join(reg.dir, serverFile), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	return ep, fmt.Sprint(cmd.Process.Pid), done
}

// A drvu of the user that lost its last tree unseen is named as such, with
// its PID and a command that stops it, never as the address itself.
func TestTakenNamesStrayServer(t *testing.T) {
	ep, pid, _ := stray(t)
	msg := taken(ep)
	if strings.Contains(msg, "taken by "+ep.String()) {
		t.Errorf("names the address as its own holder: %s", msg)
	}
	for _, want := range []string{"pid " + pid, "no live tree", "--kill -a " + ep.String()} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q lacks %q", msg, want)
		}
	}
}

// A listener on the address that is no drvu of the user is not named as
// the address itself either.
func TestTakenByStranger(t *testing.T) {
	e := testEndpoint(t)
	ln, err := net.Listen("tcp", e.String())
	if err != nil {
		t.Skip(err)
	}
	defer ln.Close()
	msg := taken(e)
	if strings.Contains(msg, "taken by "+e.String()) || !strings.Contains(msg, "not a "+progName+" of yours") {
		t.Errorf("message %q", msg)
	}
}

// --kill on the address stops a drvu that serves no live tree.
func TestKillStopsStrayServer(t *testing.T) {
	ep, _, done := stray(t)
	if code := kill(config{ep: ep, addr: ep.String()}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the stray server was not signalled")
	}
}
