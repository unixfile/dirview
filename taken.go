package dirview

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// taken explains why ep cannot be bound when no drvu of this user serves
// a live tree there: it names the listeners that hold the port, marking
// the ones that are drvu servers on other addresses. A drvu of the user
// that holds ep with no live tree, eg after its tree's lock went
// unseen, is named with its PID and the command that stops it.
func taken(ep netip.AddrPort) string {
	msg := ep.String() + " is taken"
	if srv, ok := strayServer(ep); ok {
		return fmt.Sprintf("%s by a %s of yours, pid %d, that serves no live tree; stop it with %s --kill %s",
			msg, progName, srv.pid, progName, addrFlag(ep))
	}
	var names []string
	self := false
	for _, l := range holders(ep) {
		switch {
		case len(peekTrees(l)) > 0:
			names = append(names, "the "+progName+" on "+l.String())
		case l == ep:
			// Named as the holder, ep itself would say nothing.
			self = true
		default:
			names = append(names, l.String())
		}
	}
	if self {
		names = append(names, "a program that is not a "+progName+" of yours")
	}
	if len(names) > 0 {
		msg += " by " + strings.Join(names, ", ")
	} else {
		msg += ", and no " + progName + " of yours serves there"
	}
	if ep.Addr().IsUnspecified() {
		msg += ". 0.0.0.0 claims the port on every address; share on one address, eg --share, or on another port, eg --share=0.0.0.0:8081"
	}
	return msg
}

// strayServer returns the server that the registry of ep names when it
// runs, serves no live tree and something listens on ep. A PID from
// another namespace cannot be checked or stopped from here, so such a
// server is never stray.
func strayServer(ep netip.AddrPort) (lockInfo, bool) {
	srv, ok := serverOf(ep)
	if !ok || srv.foreign() || len(peekTrees(ep)) > 0 || len(holders(ep)) == 0 {
		return lockInfo{}, false
	}
	return srv, true
}

// addrFlag is the flag that names ep: -a for loopback, --share beyond it.
func addrFlag(ep netip.AddrPort) string {
	if ep.Addr().IsLoopback() {
		return "-a " + ep.String()
	}
	return "--share=" + ep.String()
}

// holders returns the TCP listeners whose address keeps ep from binding:
// one on the same address, or on either side a wildcard on the same port.
func holders(ep netip.AddrPort) []netip.AddrPort {
	var out []netip.AddrPort
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, l := range listeners(string(b)) {
			if l.Port() == ep.Port() && (ep.Addr().IsUnspecified() || l.Addr().IsUnspecified() || l.Addr() == ep.Addr()) {
				out = append(out, l)
			}
		}
	}
	return out
}

// listeners reads the sockets in LISTEN state from a /proc/net/tcp or tcp6
// table.
func listeners(table string) []netip.AddrPort {
	var out []netip.AddrPort
	for _, sk := range sockets(table) {
		if sk.state == "0A" {
			out = append(out, sk.local)
		}
	}
	return out
}

// socket is one row of a /proc/net/tcp or tcp6 table.
type socket struct {
	local, remote netip.AddrPort
	state         string // in hex, eg 0A for LISTEN
	uid           int
}

// sockets reads the rows of a /proc/net/tcp or tcp6 table.
func sockets(table string) []socket {
	var out []socket
	for _, line := range strings.Split(table, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		local, ok1 := procAddr(f[1])
		remote, ok2 := procAddr(f[2])
		if !ok1 || !ok2 {
			continue
		}
		uid := -1
		if len(f) >= 8 {
			if n, err := strconv.Atoi(f[7]); err == nil {
				uid = n
			}
		}
		out = append(out, socket{local: local, remote: remote, state: f[3], uid: uid})
	}
	return out
}

// procAddr parses an address of a /proc/net/tcp table, eg 0100007F:1F90.
// The kernel prints each 32-bit word of an address in host order.
func procAddr(s string) (netip.AddrPort, bool) {
	addr, port, ok := strings.Cut(s, ":")
	raw, err := hex.DecodeString(addr)
	n, perr := strconv.ParseUint(port, 16, 16)
	if !ok || err != nil || perr != nil || (len(raw) != 4 && len(raw) != 16) {
		return netip.AddrPort{}, false
	}
	for i := 0; i < len(raw); i += 4 {
		binary.NativeEndian.PutUint32(raw[i:], binary.BigEndian.Uint32(raw[i:]))
	}
	a, _ := netip.AddrFromSlice(raw)
	return netip.AddrPortFrom(a.Unmap(), uint16(n)), true
}
