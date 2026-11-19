package dirview

import "testing"

// A watch on a network mount makes the watcher poll until it goes, and a
// local directory does not.
func TestWatcherPollsRemote(t *testing.T) {
	local, remote := t.TempDir(), t.TempDir()
	old := remoteFS
	remoteFS = func(d string) (string, bool) { return "NFS", d == remote }
	t.Cleanup(func() { remoteFS = old })
	w := newWatcher(lockMask)
	defer w.close()
	if w.add(local); w.polling() {
		t.Fatal("polls for a local directory")
	}
	wd := w.add(remote)
	if !w.polling() {
		t.Fatal("does not poll for a directory on a network mount")
	}
	w.remove(wd)
	if w.polling() {
		t.Fatal("polls after the watch on a network mount went")
	}
}
