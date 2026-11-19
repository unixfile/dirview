# dirview

dirview reads directories and repos in the browser: Markdown as pages, code as code, history beside both. Its command is `drvu`.

It is for anyone who keeps notes, docs and code in plain files and wants to read them as linked pages. It runs on your machine. Nothing needs a build step, and nothing needs a push first.

A repo on GitHub, GitLab or Codeberg reads the same way, once drvu has fetched it.

It is fast. A page takes one to three milliseconds and a server starts in about 40 ms. `make test` fails any page that takes over 5 ms.

## Usage

```
drvu [-p] [--foreground] [--no-open] [-a ADDR] [--debug] [--no-bug]
    [--cache=SIZE] [--disk=SIZE] [--base=PATH] [--] [DIR]
drvu [-p] [--foreground] [--no-open] --share[=HOST] [--secret] [--debug]
    [--no-bug] [--cache=SIZE] [--disk=SIZE] [--base=PATH] [--] [DIR]
drvu --restart [-f] [--no-open] [--debug] [--no-bug]
    [-a ADDR | --share[=HOST]] [--] [DIR]
drvu -l [-a ADDR | --share[=HOST]] [--] [DIR]
drvu --kill [-a ADDR | --share[=HOST]]
drvu --commit=REV [--] [DIR]
drvu -h | --help
drvu -V | --version
```

A second executable, `drvu-public`, serves online repos to the web and nothing else:

```
drvu-public --disk=SIZE (-a ADDR | --share[=HOST]) [--debug] [--cache=SIZE]
    [--base=PATH]
drvu-public -h | --help
drvu-public -V | --version
```

Run `drvu` in a directory. It prints the tree's URL and opens it with `xdg-open`, a tab in your default browser. To open it another way, set `DRVU_OPEN`, eg `DRVU_OPEN='brave-browser --app=%s'` for a window of its own; [options](doc/options.md) has more.

It also writes a lockfile named after the endpoint, eg `127.31.1.1-8080.lock`. Delete the lockfile to stop serving the tree. Trees on one address share one server, which exits when its last lock is gone. The server on your default address stays up, since it also serves [online repos](doc/online-repos.md).

```
drvu                         serve this directory and open it
drvu -p ~/notes              serve ~/notes, and again after a reboot
drvu --share=192.168.1.5     serve this directory to the LAN
drvu -l                      list every served tree as keyval
drvu --restart               move every server to the current binary
drvu-public --disk=6G -a 127.0.0.1:8080
                            serve online repos to the web, behind a proxy
```

Add `?log` to any page's URL for its history, or `?log=rss` for the same as a feed.

To read a repo from GitHub, open `http://127.31.1.2/github.com/junegunn/fzf/`.

## Install

drvu needs git.

`make install` links `build/drvu` and `build/drvu-public` into `~/.local/bin` and serves on port 8080.

`make install-deb` builds and installs two Debian packages: dirview and dirview-syntax, its syntax tree helper. The package's binary may bind port 80, so URLs carry no port.

## Documentation

- [Options and environment](doc/options.md): every option, the keyval output and the variables that set defaults
- [Servers and locks](doc/servers.md): how trees join a server, the index at `/`, addresses, restarts, serving after reboot and a base path
- [Pages](doc/pages.md): URLs, how docs and listings render, links, live reload and the look
- [Code pages](doc/code-pages.md): highlighting, line links, scope boxes, the syntax tree, gron and the switch between JSON, YAML and keyval
- [Search and controls](doc/search.md): the search box, keys, the `:` command list, the copy buttons and when controls fade
- [History](doc/history.md): histories, their feeds, commits, stardates, the timeline and old versions of a tree
- [Online repos](doc/online-repos.md): reading a forge's repos on the second address, or serving them to the web
- [Sharing](doc/sharing.md): serving a tree to other machines
- [Safety](doc/safety.md): what drvu refuses to serve, follow or run
- [Speed](doc/speed.md): what drvu caches and how it is timed
- [Log](doc/log.md): what drvu writes to the journal
- [Debugging](doc/debugging.md): the bug button and the build stamp
- [Install](doc/install.md): the Debian package, port 80 and locks in git
- [Development](doc/development.md): `make run`, the two executables, the working name and the usage check
- [Testing](doc/testing.md): Go tests, the black-box suite and the heavy runs

## Licence

MIT, see `LICENSE`.

The embedded fonts are subsets of STIX Two Text and Iosevka, under the SIL Open Font License: see `assets/fonts/STIXTwoText-OFL.txt` and `assets/fonts/Iosevka-OFL.txt`.

The binary links in five Go modules. Four are under the MIT licence: goldmark by Yusuke Inuzuka, chroma by Alec Thomas, regexp2 by Doug Clark and kv, dirview's sister project. regexp2 ports parts of Microsoft's dotnet/corefx. The fifth, yaml.v3 by Canonical, is under Apache 2.0, with its files from Kirill Simonov's libyaml under MIT. The package's copyright file carries their notices.
