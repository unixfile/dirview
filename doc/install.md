# Install

drvu needs git.

Saving a doc as PDF also needs typst. No Debian suite packages it, so install a release binary from https://github.com/typst/typst/releases, eg into `/usr/local/bin`, or build it with `cargo install --locked typst-cli`. The server must find it on its PATH, and the PDF's fonts, STIX Two Text and Iosevka, among the machine's fonts.

`make install` links `build/drvu` into `~/.local/bin`.

## Debian packages

`make deb` builds a Debian package in `build/`, such as `dirview_0~git20261015223930.20f02fd_amd64.deb`, versioned by the last commit's UTC time and hash. It depends on git, and not on typst. For another architecture, pass `ARCH`, eg `make deb ARCH=arm64`.

`make deb-syntax` builds the syntax tree helper `drvu-syntax` as a package of its own beside it, `dirview-syntax`, since the helper needs cgo and libc while drvu is static. The main package recommends it and the helper's package depends on dirview, each at the same version, because drvu reads the helper's output. For another architecture it needs a C cross compiler, eg `make deb-syntax ARCH=arm64 CC=aarch64-linux-gnu-gcc`.

`make install-deb` builds both packages while sudo asks for the password, then installs them with `sudo apt install`, which pulls in git. It first removes the `~/.local/bin` link from `make install`, which would otherwise shadow the package's binary. Run it again to replace an installed build, even of the same commit. `make uninstall-deb` removes both.

Debian's needrestart lists the services that use old libraries after the install instead of asking which to restart. None of them is drvu.

## Port 80

The package holds both executables, `/usr/bin/drvu` and `/usr/bin/drvu-public`. It grants `/usr/bin/drvu` one capability, `cap_net_bind_service`, so it can bind port 80 while running as the user. `getcap /usr/bin/drvu` shows it.

With it drvu serves on port 80 and URLs carry no port. Without it, as after `make install`, the port is 8080. drvu also takes port 80 where the machine lets every user bind low ports.

## Locks in git

Add `*.*.*.*-*.lock` and `*.*.*.*-*.LOCK` to `~/.config/git/ignore` so locks stay out of `git status`.
