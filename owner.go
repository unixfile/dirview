package www

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"os"
)

// The reader's web address answers only the account that runs it. Every
// account on the machine reaches loopback, and the address serves
// private repos read with the user's gh login, so a connection from
// another account, or one whose account cannot be told, gets 403. The
// kernel names each socket's owner in /proc/net/tcp; the client's socket
// there is the one whose local end is the connection's remote end.

type peerKey struct{}

// peerUID records in the connection's context which account opened c,
// or -1 when /proc cannot say.
func peerUID(ctx context.Context, c net.Conn) context.Context {
	return context.WithValue(ctx, peerKey{}, connUID(c))
}

// connUID is the account that owns the far end of c, a connection on
// this machine, or -1.
func connUID(c net.Conn) int {
	local, err1 := netip.ParseAddrPort(c.LocalAddr().String())
	remote, err2 := netip.ParseAddrPort(c.RemoteAddr().String())
	if err1 != nil || err2 != nil {
		return -1
	}
	local, remote = netip.AddrPortFrom(local.Addr().Unmap(), local.Port()), netip.AddrPortFrom(remote.Addr().Unmap(), remote.Port())
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, sk := range sockets(string(b)) {
			if sk.local == remote && sk.remote == local {
				return sk.uid
			}
		}
	}
	return -1
}

// ownerOnly answers 403 to a request on a connection that another
// account opened, or one whose account peerUID could not tell.
func ownerOnly(next http.Handler) http.Handler {
	uid := os.Getuid()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, ok := r.Context().Value(peerKey{}).(int); !ok || got != uid {
			http.Error(w, "This address serves only the account that runs its server.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
