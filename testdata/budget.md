# www

www reads directories and repos in the browser: Markdown as pages, code as code, history beside both.

It is for anyone who keeps notes, docs and code in plain files and wants to read them as linked pages. It runs on your machine. Nothing needs a build step, and nothing needs a push first.

A repo on GitHub, GitLab or Codeberg reads the same way, once www has fetched it.

It is fast. A page takes one to three milliseconds and a server starts in about 40 ms. `make test` fails any page that takes over 5 ms.

## Usage

```
www [-p] [--foreground] [--no-open] [-a ADDR] [--debug] [--no-bug]
    [--cache=SIZE] [--disk=SIZE] [--base=PATH] [--] [DIR]
www [-p] [--foreground] [--no-open] --share[=HOST] [--secret] [--debug]
    [--no-bug] [--cache=SIZE] [--disk=SIZE] [--base=PATH] [--] [DIR]
www --restart [-f] [--no-open] [--debug] [--no-bug]
    [-a ADDR | --share[=HOST]] [--] [DIR]
www -l [-a ADDR | --share[=HOST]] [--] [DIR]
www --kill [-a ADDR | --share[=HOST]]
www --commit=REV [--] [DIR]
www -h | --help
www -V | --version
```

A second executable, `www-public`, serves online repos to the web and nothing else:

```
www-public --disk=SIZE (-a ADDR | --share[=HOST]) [--debug] [--cache=SIZE]
    [--base=PATH]
www-public -h | --help
www-public -V | --version
```

Run `www` in a directory. It prints the tree's URL and opens the browser.

It also writes a lockfile named after the endpoint, eg `127.31.1.1-8080.lock`. Delete the lockfile to stop serving the tree. Trees on one address share one server, which exits when its last lock is gone.

```
www                         serve this directory and open it
www -p ~/notes              serve ~/notes, and again after a reboot
www --share=192.168.1.5     serve this directory to the LAN
www -l                      list every served tree as keyval
www --restart               move every server to the current binary
www-public --disk=6G -a 127.0.0.1:8080
                            serve online repos to the web, behind a proxy
```

Add `?log` to any page's URL for its history.

To read a repo from GitHub, open `http://127.31.1.2/github.com/junegunn/fzf/`.

## Install

www needs git.

`make install` links `build/www` and `build/www-public` into `~/.local/bin` and serves on port 8080.

`make install-deb` builds and installs two Debian packages: www and its syntax tree helper. The package's binary may bind port 80, so URLs carry no port.

## Documentation

- [Options and environment](doc/options.md): every option, the keyval output and the variables that set defaults
- [Servers and locks](doc/servers.md): how trees join a server, the index at `/`, addresses, restarts, serving after reboot and a base path
- [Pages](doc/pages.md): URLs, how docs and listings render, links, live reload and the look
- [Code pages](doc/code-pages.md): highlighting, line links, scope boxes, the syntax tree, gron and the switch between JSON, YAML and keyval
- [Search and controls](doc/search.md): the search box, keys, the `:` command list, the copy buttons and when controls fade
- [History](doc/history.md): histories, commits, stardates, the timeline and old versions of a tree
- [Online repos](doc/online-repos.md): reading a forge's repos on the second address, or serving them to the web
- [Sharing](doc/sharing.md): serving a tree to other machines
- [Safety](doc/safety.md): what www refuses to serve, follow or run
- [Speed](doc/speed.md): what www caches and how it is timed
- [Log](doc/log.md): what www writes to the journal
- [Debugging](doc/debugging.md): the bug button and the build stamp
- [Install](doc/install.md): the Debian package, port 80 and locks in git
- [Development](doc/development.md): `make run`, the two executables, the working name and the usage check
- [Testing](doc/testing.md): Go tests, the black-box suite and the heavy runs

## Licence

MIT, see `LICENSE`.

The embedded fonts are subsets of STIX Two Text and Iosevka, under the SIL Open Font License: see `assets/fonts/STIXTwoText-OFL.txt` and `assets/fonts/Iosevka-OFL.txt`.

The binary links in five Go modules. Four are under the MIT licence: goldmark by Yusuke Inuzuka, chroma by Alec Thomas, regexp2 by Doug Clark and kv, www's sister project. regexp2 ports parts of Microsoft's dotnet/corefx. The fifth, yaml.v3 by Canonical, is under Apache 2.0, with its files from Kirill Simonov's libyaml under MIT. The package's copyright file carries their notices.

# Code pages

Code in a fenced block with a known language is highlighted by chroma, as are code pages, whose language follows the file name.

## Highlighting speed

Go is lexed by Go's own scanner from the standard library instead, about 150 times faster, with the same colours. A Go page of 1,400 lines takes 2 ms.

Python, JavaScript and shell have lexers of www's own. They follow chroma's rules and give every byte the class chroma gives it. chroma took 180 ms to lex argparse.py, 2,633 lines, and they take under 2 ms. A whole page of it renders in about 3 ms, and jquery.js, 10,907 lines, in about 20 ms instead of 400.

Other languages go through chroma, at about 70 µs a line.

The server keeps each coloured page by its content's hash, within 16 MB, so a later view takes a few milliseconds.

## Line links

A code page anchors lines as GitHub does, so `main.go#L42` lands on line 42 and highlights it. A range such as `main.go#L10-L20` highlights those lines.

Clicking a line's number highlights it and puts its address in the address bar. Clicking the number again clears both.

## Views

A code page's views, flat, scopes and tree, and raw sit in a row of their own above the code's top right corner. The row follows the page down as it scrolls. Each view keeps its place; the view shown is not a link and stands in reverse video, light on dark where the page is dark on light.

## Scopes

Add `?scopes` to a code page, or follow scopes, and it draws each block of code that spans lines as a sunken box behind its text. Boxes nest as deep as the code goes, and deeper boxes are fainter.

A block is a bracket pair whose ends sit on different lines, or in Python the lines indented under a line that ends with a colon. Brackets and strings within a line, around something, are smaller boxes inside it, down to the innermost, much as vim's `ci(` and `ci"` see them. A string that spans lines is a block. A box starts at its least indented line, so a raw string's lines at the margin stay inside it.

Fills alternate inward between a light shade and the page's colour, so depth shows without the page darkening. The boxes lie on a layer of their own, so no character moves and copied text is unchanged.

A box is a link to its lines. The box under the pointer is tinted. A click marks it and the boxes in it and puts its lines in the address, eg `#L482-L520`; a click on the marked box clears both.

When the file is committed and unchanged since, the address also names that commit's version, eg `@261024.22/server.go?scopes#L3-L26`. The link then keeps pointing at the same lines after the file changes. Only the address moves: the page's links, eg raw and tree, still open the current file, and a live reload goes back to it.

In the same corner, flat goes back to the plain page.

## Syntax tree

Add `?tree`, or follow tree, for the syntax tree: one row per named node, with its field in its parent, its kind, its text on one line and a link to its lines. It is folded so the file's top-level declarations show first.

A helper parses it with tree-sitter: `www-syntax`, which the package of that name installs and `make syntax` builds beside www with a C compiler. It knows Go, Python, JavaScript, TypeScript, Rust, C, C++, shell, Java, C#, JSON, HTML, CSS and Ruby. Without the helper, or for another language, the page says so above the plain code.

A first view of a 1,400-line file takes about 20 ms and sends 1.2 MB; one of jquery.js, 10,900 lines, takes about 85 ms, most of it in the parse. www renders the rows as the helper prints them. The server keeps the tree, so a later view takes a few milliseconds.

## Gron

A JSON file has one more view, gron, at the left end of that row. `?gron` shows the file as one line per node, each a JavaScript assignment such as `json.tags[0] = "a";`, so every value reads and searches with its full path.

Keys keep the file's order and values show as written. A file that is not valid JSON shows the plain code under a line that names where parsing stopped.

## Formats

A JSON, YAML or keyval file links the other two formats at the left end of the row, as `as json`, `as yaml` or `as keyval`. Each link, `?json`, `?yaml` or `?keyval`, shows the same document converted to that format. The row does not name the file's own format: flat already shows the file as written, and leads back to it from a converted view. The files are those ending in `.json`, `.yaml`, `.yml` and `.kv`.

[kv](https://github.com/unixfile/kv) converts, through keyval's tree. A converted view is a reading aid, not the file: a line above the code says what changed. Keys are sorted. An empty list becomes an empty map. From YAML, comments are dropped and aliases are written out, and a stream of documents becomes a list.

Values keep their types. A string that looks like a number or `true` stays quoted, and a name with a dot converts whole.

Some files do not convert, and the page then says why above the file as written: a file that does not parse, a document that is a single value, a name with a space, or YAML whose aliases expand past a million nodes.

A converted view takes about a second per MB the first time, most of it in kv and in colouring YAML, so a file over 1 MB is not converted. The server keeps the view by the file's hash, and a later view takes a few milliseconds.

# Debugging

## Bug reports

A small, faint `www bug` button sits at the foot of every page. It reports a bug in www and the server, not in the tree you are reading.

A click files a numbered bug with the page attached, and the button says which, eg `bug 7 filed`. Text selected on the page goes in as the note. A field then opens beside the button for a comment: Enter adds it, Escape or clicking away leaves the bug without one.

Each report lands in `csi/bugs/` of the checkout the binary was built from, when `make build` built it there and that checkout has csi, so the bugs sit with www's own project. Otherwise, as for the package, it lands in `~/.local/state/www/bugs/`, or below `$XDG_STATE_HOME` when set:

- `7.kv`: the report as keyval: the time, the page's URL, the note, the window's size and scroll as `view`, the browser as `ua`, the build, then the server: its `pid`, its binary as `exe`, `stale yes` if a rebuild has replaced that binary, its `addr`, when it `started`, its `command` line and each tree it serves as `tree.0`, `tree.1` and on, then each comment as `comment.0`, `comment.1` and on
- `7.html`: the page as it stood at the click, script changes included, with its stylesheet and no script, so it opens as you saw it
- `next`: the number the next report gets. Numbers only grow, so a bug deleted once fixed never gives its number away

Each report and comment is also one line in the log. Find them with `journalctl --user -t www -g '^bug '`:

```
bug 7: http://127.31.1.1/x/nowhere.md note="the coin is cut off" view=1280x800+0 ua="Mozilla/5.0 ..." build=1aa282c snapshot=/home/t/scratch/github.com/unixfile/www/csi/bugs/7.html
bug 7 comment: "after a restart"
```

`--no-bug` hides the button and refuses reports. A share never shows it, so no visitor writes files on your machine.

## Build stamp

With `--debug`, each page says when its binary was built and how many seconds ago, the commit it was built from and how long the page took to render, eg `built 12:49, 108 s ago, from 1aa282c+modified, rendered in 3.3 ms`. It sits small and faint above the header, so a glance tells which build served the page.

`+modified` marks a build from uncommitted code. www's script counts the seconds up while the page is open. Shares leave the stamp out.

# Development

To try a change without installing it, run `make run`. It builds `build/www` and the syntax tree helper and serves this directory on port 8080 with `--debug`, beside an installed server on port 80.

A later run replaces the server on 8080 with the new build, serving the same trees, and open pages go on under it. So edit and `make run` again.

It opens the browser only when no page of this directory was open on the old server, and says which happened. A hidden tab holds no connection, so it does not count and gets a second tab. `www --kill -a 127.31.N.1:8080` stops it.

## Two executables

The root directory is a library package. Each executable has only its main under `cmd/`: `cmd/local` builds `www`, the reader, and `cmd/public` builds `www-public`, for the open web. `make build` builds both into `build/`.

The public executable leaves a feature out by never calling it, and the linker drops code that no call leads to. A server gets the stop form, the bug form and the index only through `local()`, which the public start never calls. `TestPublicLeavesOutTheReadersFeatures` builds both and reads their symbols: the public one must hold none of the listed names and the reader every one.

## The working name

The program's name lives in the file `WORKING_NAME`, since www is a working name until a final one is chosen. The binary, its paths, environment variables, journal tag and URLs, the Makefile and the Debian package all take it from there. A test fails on any other copy outside prose and `go.mod`.

## Usage check

The help text is written in docopt notation. `make test` checks it with docopt-ng, run through `uv`, and skips the check where `uv` is missing.

## Fonts

`make fonts` cuts the embedded fonts from STIX Two Text 2.13 and Iosevka 34.9.1, pinned by SHA-256 in `assets/fonts/subset.py`, and writes `assets/fonts.css`. It needs the network and `uv`. Its output is committed, so a build needs neither.

# History

Add `?log` to a path for its git history: the commits that touched it, newest first, one line each with its stardate and subject. A directory's history covers everything below it. A history lists the 500 newest commits.

A listing whose directory has commits opens its history from `<(git log)` on its first row. A page outside a repo shows git's complaint.

## A history's lines

The stardate leads each line, so the stardates line up, and the version menu orders its rows the same way. A commit with no message shows its day alone. Hover shows the hash and the exact time.

A line opens to the commit's full message and its diff, limited to the path. The diff loads when the line first opens. Bodies wrapped by hand join into paragraphs; a list item or an indented line keeps its own line.

A row is one target: a click anywhere on it opens or closes the fold. Inside, the full hash links the commit's own page and browse links its version.

## Stardates

Each commit shows its stardate, eg 261024.4, the fourth commit of 24 Oct 2026 on the repo's main line. The main line is the first-parent line of origin's main or master, else of the branch origin's HEAD names, eg origin/trunk, else of the local main or master.

A commit a merge brought in is named after the merge, eg 261024.4.2.

A commit on the local main or master that origin's line has not reached yet shows its day and x, eg 261026.x. Its number comes with the push, so a rebase before it renames nothing. Several commits can share a day and x, so a history or the timeline adds each one's short hash, eg 261026.x 6376dd4. Its version's address uses the hash.

A commit on no main line, eg on another branch, shows its short hash.

## Where the reader is

A doc or code page with commits opens with a line that says where the reader is: current when the page matches its last commit, changed when it has edits since. Hover names the commit by stardate and subject.

www compares git's id for the file with the one HEAD keeps, which takes microseconds, and asks `git status` only when they differ.

## The timeline

A ▸ leads the line, the browser's own sign that it opens, and turns to ▾ while open. A click anywhere on the line opens the timeline: a menu of every version of the page, newest first, then the history. `?timeline` serves it as a page.

The version shown is marked. The newest row leads to the current page and says current. When the page has uncommitted changes, a row of its own above it says so.

The menu loads when the pointer first passes over the line, so it opens full. Up and Down walk it, Enter opens a version, and Escape or a click outside closes it. On a share, where there are no versions, its rows open the commits.

## Commit pages

A commit's hash in a history's fold, or commit at the right end of a version's top line, opens the commit on a page of its own. It shows the stardate and full hash as plain text, to select and give to git, then the message and the diff. The diff is limited to the file or directory the history was for, with a link to all the files the commit changed in the tree.

Added lines sit on a green tint and removed ones on a red. The diff stops at 1 MiB.

From the terminal, `www --commit=261024.4` prints a commit's full hash, stardate and subject as keyval.

## Semantic diffs

The diff shows at once. The semantic diff follows a moment later, above the diff: each file's declarations the commit added, removed, changed, renamed, moved or only reformatted, eg `changed tree.serveSearch` or `moved Away from x.go`.

Each name opens a page of that declaration alone. It shows what the commit did to it and its diff from its old self, which a rename or a move between files follows. It links to its lines before and after where the tree has versions.

Within the diff, the tokens that changed in a changed line sit on a stronger tint, so a renamed argument marks the name alone. A run of lines that shares under half its tokens with the lines it replaced is left unmarked.

Declarations come from the syntax helper, `make syntax`, and the token marks from the lexers. Without the helper the page says so and keeps the marks.

A first view of a commit that touches 12 files took about 190 ms. The server keeps the result by commit, so a later view takes a few milliseconds and shows it with the page.

## Git's safety

www runs `git log`, `git show` and `git rev-parse` in the directory of what the page shows, after links resolve.

A cloned repo's `.git/config` can name programs for git to run. So www turns off external diff, textconv, signature checks and fetches, and drops `GIT_` variables from its environment. git's own `safe.directory` check refuses repos that other users own.

No history page shows authors, on a share or on loopback.

## On shares

A shared server shows history and diffs as loopback does, with four differences.

History appears only when other users could read the repo's git directory, its `HEAD`, `objects` and `refs`. Nobody reads through www what git run as another user would not show them.

Each message loses its trailer block, which carries emails and session links in lines such as `Signed-off-by:` and `Claude-Session:`. As in git, the last paragraph is a trailer block when a quarter of its lines are `Key: value` lines, so a cherry-pick note or a wrapped trailer does not keep it.

A page in a repo the share keeps private never reloads on a commit. Diffs leave out files whose names begin with a dot, as listings do.

A share never shows git's errors, which name paths on the machine, and says only `No git history here.`

## Old versions of a tree

Each commit in a history has a browse link, inside its fold, to the page as it was in that commit.

`@` and a commit's hash on a tree's name serve the tree as of that commit, eg `/scratch/www@5430ac3/` for its root or `/scratch/www@5430ac3/doc/history.md` for one doc. The version qualifies the tree, so it sits on the name, not below it as an entry would. A tree served at `/` has no name, so its versions sit below it, eg `/@5430ac3/`.

The address below the tree, eg `/scratch/www/@5430ac3/`, is the old form and redirects. A tree that holds an entry named like a version, eg a directory `@cafe`, serves that entry at `/scratch/www/@cafe/`, and the old form does not redirect there.

A stardate works the same, eg `www@261024.4/`, and a hash of a commit with a stardate redirects to it. A day alone, eg `www@261024/`, names no commit and says so, though its six digits would also read as a hash. Any length of hash from four characters up works and redirects to git's short form.

Docs, code pages, listings and search work there as on the tree itself, read from git rather than the disk. Relative links stay in the same version. www's script still runs there, so keys, the search box and line marks work as on the tree.

The top line names the commit that last changed the page as of that version, stardate first as in histories, and opens the same timeline with that version marked. At the line's right end, commit opens the commit's page, and current leads to the page as it is, as does the timeline's first row. The breadcrumbs and the tab title carry the hash, eg `www @ 5430ac3`.

A version never changes, so it has no live reload, no Delete lock button and no path to copy, and its docs list no backlinks.

Shares and the web address have no versions. A page whose history lies in a nested repo, or behind a link, lists its commits but no versions, since the tree's versions do not hold it.

# Install

www needs git.

`make install` links `build/www` into `~/.local/bin`.

## Debian packages

`make deb` builds a Debian package in `build/`, such as `www_0~git20261015223930.20f02fd_amd64.deb`, versioned by the last commit's UTC time and hash. It depends on git. For another architecture, pass `ARCH`, eg `make deb ARCH=arm64`.

`make deb-syntax` builds `www-syntax`, the syntax tree helper, as a package of its own beside it, since the helper needs cgo and libc while www is static. The main package recommends it and the helper depends on www, each at the same version, because www reads the helper's output. For another architecture it needs a C cross compiler, eg `make deb-syntax ARCH=arm64 CC=aarch64-linux-gnu-gcc`.

`make install-deb` builds both packages while sudo asks for the password, then installs them with `sudo apt install`, which pulls in git. It first removes the `~/.local/bin` link from `make install`, which would otherwise shadow the package's binary. Run it again to replace an installed build, even of the same commit. `make uninstall-deb` removes both.

Debian's needrestart lists the services that use old libraries after the install instead of asking which to restart. None of them is www.

## Port 80

The package holds both executables, `/usr/bin/www` and `/usr/bin/www-public`. It grants `/usr/bin/www` one capability, `cap_net_bind_service`, so it can bind port 80 while running as the user. `getcap /usr/bin/www` shows it.

With it www serves on port 80 and URLs carry no port. Without it, as after `make install`, the port is 8080. www also takes port 80 where the machine lets every user bind low ports.

## Locks in git

Add `*.*.*.*-*.lock` and `*.*.*.*-*.LOCK` to `~/.config/git/ignore` so locks stay out of `git status`.

# Log

www logs every action, always. There is no flag and no level to set, because a filter set before a mistake drops the entry that would explain it.

Entries go to the systemd journal, tagged `www`:

```
journalctl --user -t www --since -1h
journalctl --user -t www WWW_ENDPOINT=127.31.1.1-80
journalctl --user -t www -p err
```

Each entry carries the endpoint it concerns in `WWW_ENDPOINT` and its priority in `PRIORITY`: 3 for faults, 6 for the rest. The journal also holds what explains a vanished server, such as a suspend, an OOM kill or a logout.

Where no journal socket exists, entries go to `~/.local/state/www/log` instead, or below `$XDG_STATE_HOME` when set. Its lines hold time, PID, endpoint, priority and the message. The file is mode 600 and rotates once to `log.1` at 1 MB. `www --help` names the place in use.

## What is logged

- Every run of `www`, with its arguments, working directory and binary, and its outcome when it fails. A run that joins a server or finds its tree already served says so.
- Each server's start with its binary and commit, eg `/usr/bin/www 54cca2a`, and `+modified` for a build from uncommitted code. A stale binary is the first suspect when a page looks wrong.
- Each tree served and withdrawn, with the cause: a deleted lock, a Delete lock or Kill server button, or a signal. Then the server's stop and exit.
- Stale locks and registry links cleared after a crash, and the SIGTERM that `--kill` and `--restart` send.
- Every page request, with method, path, status and time taken. Live reload streams, the stylesheet, fonts and the favicon come with every page and are left out. A shared server adds the visitor's address.
- Bug reports from the bug button at each page's foot, as one line starting `bug N:` with the page's URL, the note, the window, the browser, the build and the snapshot's path, and each comment as `bug N comment:`.
- Faults: render and git failures, refused links, inotify falling back to polling, and what net/http reports. A detached server's stderr goes to the same place, so a panic is kept.

## Privacy

A secret share's token never reaches the log. Paths do, and on shares so do visitors' addresses.

Your user journal is readable by you and by members of `adm` and `systemd-journal`.

# Online repos

The server on your default address also listens on the address after it, 127.31.N.2, on the same port. There a URL names a repo on a forge by its host and path, eg `http://127.31.1.2/github.com/junegunn/fzf/README.md`. The first visit fetches the repo into a cache. From then on www serves it as it serves a checkout: docs, code, listings, search and history.

Home and the web never collide, since each has an address of its own.

The index at `/` on the default address links the web address. The web address's own `/` lists the repos in the cache, each with its size and when it was last read. It says how much of the disk limit they fill, and links back to the trees on the default address.

## Ways in

The index at `/` names the web address, but a reader seldom sees the index, so two ways in lie where a reader already is.

A link in a doc to a repo on GitHub, GitLab, Codeberg or Bitbucket, or to a file or directory in one, is followed by a small www, raised like a footnote mark. It opens the same repo or file on the web address, and its title names the repo. The forge's own link stays as it was.

On the web address the link turns around. A reader there is already inside an online repo, so a link to another repo opens it in www, and the mark after it names the forge, eg github, and opens the forge's page. A link in your own docs still goes where it says.

A link to a forge page that is not a file, eg an issue or an account, gets no mark. A link to a file's bare bytes counts as a link to the file, eg on raw.githubusercontent.com. A link pinned to a commit by its hash opens that version, eg `fzf@26ff936`. A branch or tag drops, since the web address serves the commit it fetched. The copy button leaves the mark out.

A forge URL typed or pasted into the search box, with or without `https://`, adds a result above the others, under "online". Enter opens it, since it comes first.

A URL to any other host, eg `https://git.suckless.org/dwm`, gets no result there; paste it after the web address instead, scheme and all, eg `http://127.31.1.2/https://git.suckless.org/dwm`. The web address drops the scheme, and turns a forge's view of a file, eg `/blob/master/README.md`, into the file.

Both appear wherever a server serves the web address: on its default address and on the web address itself. A share offers neither.

## The first visit

A repo not yet cached opens a page that says "reading". It names each step as www takes it: the git command, the URL and where the files go, with git's progress below and its time. When the repo is ready the page turns into it on its own. For most repos that takes one to three seconds; git's own repo takes about 20.

www first finds where the repo ends in the path. It asks `git ls-remote` on every prefix at once, with and without `.git`, and takes the longest that answers; the rest of the path lies inside the repo. GitHub, Bitbucket and Codeberg keep every repo at owner/name, so there it asks nothing and clones at once.

A URL that ends in `.git` redirects to the one without, so a repo has one URL.

When another website links to a repo not yet cached, the browser marks the request as cross-site, and www asks before fetching anything. The page's read link keeps the link's fragment, eg the lines `#L10-L12` marks, and the page never reloads on its own, so nothing is fetched until you follow it. A URL typed, bookmarked or followed from a www page fetches at once. So no site can make your browser fill the disk.

## The cache

The cache lives in `~/.cache/www/`, or `$XDG_CACHE_HOME/www/`, one directory per repo named by its host and path. Each holds a bare clone without file contents, in `.git`, and one worktree of the commit served, such as `.wt-b412a450538e`, which the link `.tree` names.

The worktree's checkout fetches every file of that commit in one request. Older file versions arrive when a page needs them: a commit's diff fetches the blobs it shows, which takes about half a second on GitHub. A version fetches a directory's files in one request, the first time its listing or one of its files opens, so its first page takes about the same.

`--disk=SIZE` sets the cache's limit, 2G by default. When a fetch takes it over, www deletes the repos read longest ago until it fits. A repo larger than the limit on its own goes too, and its page says so.

Delete the directory any time; www fetches what it needs again.

## Staying current

A visit to a repo fetched over five minutes ago serves the cached commit at once and fetches in the background. When the branch moved, www builds the new commit's worktree beside the old one and swaps the link, so no page sees a half-updated tree. Open pages reload onto the new commit.

The header of every page in the repo names the commit shown by its stardate, eg `@ 261029.3`, as a version's crumbs name a commit. Its hash and time show on hover. The stardate links the page in that commit's version, which stays put after the next fetch. A commit off the main line shows its hash instead.

The header gives no fetch time while fetches work, since a visit fetches any copy over five minutes old. It says `fetching` while a fetch runs. When a fetch fails, the page keeps the commit it has and says when the copy was fetched and when the fetch failed, eg `@ 261029.3 · fetched 09:12 · fetch at 17:17 failed`.

A fetch takes the branches and tags. It leaves out GitHub's pull request refs, which would fill a large repo's cache with commits nobody asked for.

## Versions and links

A repo here has versions as a local tree has: `@` and a commit below the repo's address, eg `/github.com/junegunn/fzf@b412a45/main.go`, shows the repo as it was then, and its pages, history and share button link them. The clone holds every commit and tree, so a version's listings come at once; its files are fetched from the forge as a page first needs them.

The share button here copies the forge's own link at the commit the page shows, eg `https://github.com/junegunn/fzf/blob/<commit>/main.go#L10-L12`, since a link to this address opens only on this machine. Lines take the forge's form: `#L10-L12` on GitHub and Codeberg, `#L10-12` on GitLab and `#lines-10:12` on Bitbucket. A host www knows no link form for gets the page's own address.

## Listings

A path that names no repo lists the repos below it, eg `/github.com/junegunn` or `/git.suckless.org`. git cannot list an account, so here alone www asks something else, in this order:

- the forge's API: GitHub, GitLab, Bitbucket, and Forgejo or Gitea on any other host, such as Codeberg
- a web frontend's index: gitweb's project index, or the front page of cgit or stagit
- the cache, which holds only what was read here

The listing names its source and the sources that failed first, so a partial listing does not pass for a whole one. It is kept for an hour, since GitHub allows 60 calls an hour without a token.

## On the open web

`www-public --disk=SIZE -a ADDR` serves online repos to anyone who reaches the address, eg through Caddy in front of it. It serves no tree from the machine and no other address, and it runs in the foreground for a service manager. `--share=HOST` binds a public address directly instead. `--base=/www` puts every URL below `/www`, for a proxy that gives www only that part of a domain; the proxy passes the path on whole.

`www-public` is an executable of its own, built from the code `www` uses for online repos. It does not contain the bug button, the stop buttons, the index of servers, the browser launch, `--restart`, `-l`, `--kill`, `-p` or locks, so no flag or request can reach them. `make test` reads the binary's symbols and fails if one of them is there. `www --public` is gone and says where it went.

A visitor learns nothing of the machine or of what others read. `/` says only how to read a repo. A listing that no forge or web frontend gives is a 404, never the cache. Pages write the cache's paths from `cache/`, git's errors included. 

No page has a stop or bug button, nothing opens a browser, and any Host header is answered, since nothing local is there to protect.

`--disk` is required, since any visitor can make www clone. A clone, fetch or checkout stops once the repo outgrows it or after 10 minutes, so a forge that sends without end cannot fill the disk or hold a clone's place.

A visitor can name any host, so www fetches only from hosts whose every address is on the open internet: never loopback, a private or link-local range, or carrier-grade NAT. git connects only to the addresses checked and follows no redirect, and listings check each address they connect to. A repo's `.git` is hidden, and a link out of a repo is refused without saying where it leads, whatever marker lies there.

The limits aim at a bot that makes www clone without end, not at a reader. A repo already in the cache opens at any time. A visit to an uncached path starts a read: a probe, a clone or a listing.

One address runs one read at a time and starts at most 20 within an hour. Over that it gets a 429 with `Retry-After`, and the log names the address. An IPv6 address counts by its /64. Behind a proxy on loopback, eg Caddy, the visitor is the last address in `X-Forwarded-For`.

At most four clones run at once, and a fifth waits on its reading page.

One leak remains: a repo already in the cache opens at once, and one that is not shows the reading page first.

## What git may do

Every git call against a forge runs without your credentials, without your git config and without prompts or hooks, and only over HTTPS. A forge that asks for a login gets none, so only public repos open. Loopback is open to every account on the machine, and a private repo fetched here would be open to them too.

Elsewhere git still may not fetch at all. git 2.44 and later honour `GIT_NO_LAZY_FETCH`; before it, `protocol.allow=never` alone holds that line.

A share never serves the web address.

# Options and environment

`www --help` prints the options in short. Here is each in full.

- `-a ADDR`, `--addr=ADDR`: serve on another loopback address, an IP or IP:PORT; the port defaults to 80 where www may bind it, else 8080
- `--share[=HOST]`: share the tree on the default route's address, or on the address or name given beyond loopback, with an optional port, such as `192.168.1.5`, `0.0.0.0` or `mate.peer.home`; see `doc/sharing.md`
- `--secret`: serve a shared tree under the token in `WWW_SECRET`, or a random one, so only those holding the link reach it
- `--foreground`: when no server runs on the address yet, serve in the foreground until Ctrl-C
- `--no-open`: print the URL without opening a browser
- `--no-bug`: hide the bug button at the foot of each page and refuse reports; `--restart` passes it to the new servers when given
- `-p`, `--persist`: write a persistent lock, such as `127.31.1.1-8080.LOCK`, so the tree is served again after a reboot; see `doc/servers.md`
- `--debug`: show what serves debugging: above each page's header, the build stamp, see `doc/debugging.md`; on the index, each server's build and start time and each lock's time. `make run` sets it; `--restart` passes it to the new servers when given, and leaves it off otherwise
- `--cache=SIZE`: memory a new server may spend on cached git answers, eg `64M`; `0` caches nothing. The default is `32M`, and `--restart` goes back to it
- `--disk=SIZE`: disk a new server may fill with online repos, eg `4G`; it drops the repos read longest ago, but not one that another server is fetching, since every server shares the cache. The default is `2G`, and `--restart` goes back to it
- `--base=PATH`: serve a new server's every URL below PATH, eg `--base=/www` for `http://127.31.1.1/www/notes/`; see `doc/servers.md`
- `--restart`: replace every server this binary started with one running it now, serving the same trees, and print each tree's URL; an address with persistent trees and no server gets one, when this binary served it last; with `-a` or `--share`, only the server on that address, whichever binary runs it; with a directory, it also serves that directory there and opens it in the browser unless a page of it was open, and says which
- `-f`, `--force`: with `--restart`, take the servers and persistent trees of other binaries too
- `-l`, `--list`: print each served tree's directory, URL and binary as keyval, on every address; with `-a` or `--share`, on that address only; with a directory, only the trees at or below it. A directory that holds no served tree prints nothing; one that does not exist fails with `E2`
- `--kill`: stop every server and print the trees it served, as `--list` does; with `-a` or `--share`, only the server on that address
- `--commit=REV`: print as keyval the commit REV names in the repo of DIR, or of the current directory: its full hash, its stardate and its subject. REV is a stardate, eg `261024.4`, a hash or any name git takes, so `git show $(www --commit=261024.4 | sed -n 's/^hash //p')` shows a commit seen on a page. A day and x, eg `261026.x`, works while one commit alone holds it. A commit or repo that is not there fails with `E2`
- `-V`, `--version`: print when the binary was built and the commit it came from, as the stamp says it, eg `www built 2026-10-21 12:49, from 1aa282c+modified`
- `-h`, `--help`: print the options

`www-public` takes six of these: `-a`, `--share`, `--disk`, which it requires, `--cache`, `--debug` and `--base`, with `-V` and `-h`. It takes no directory. `www-public --help` prints them, and `doc/online-repos.md` says what it serves.

Options follow GNU style. Short ones combine, as in `-pa 127.31.1.2`, and options may follow the directory. Optional values go joined, as in `--share=192.168.1.5`; with a space the address would be read as the directory, and www says so. `-f` and `-n` mean force and dry run in every tool of the author's, so `--foreground` and `--no-open` have no short form. The help text is written in docopt notation, except that docopt has no way to write the optional `=HOST`.

`--list` and `--kill` print [keyval](https://unixfile.github.io/keyval/): three lines per tree, sorted, with every index the same width. The binary line is missing for a server that started before www recorded it.

```
tree.0.binary /usr/bin/www
tree.0.dir /home/t/keep/notes
tree.0.url http://127.31.1.1/keep/notes/
tree.1.binary /home/t/scratch/github.com/unixfile/www/build/www
tree.1.dir /home/t/scratch/github.com/unixfile/www
tree.1.url http://127.31.1.1:8080/scratch/github.com/unixfile/www/
```

Two variables set the default addresses. `--help` shows each value and where it comes from, and an error from a bad value names its variable.

- `WWW_SHARE`: the address or name for a bare `--share`, eg `mate.peer.home`, in a start and in `--kill`, `--restart` and `--list`. Without it a bare `--share` takes the default route's address
- `WWW_ADDR`: the loopback address for a start without `-a`. `--list`, `--kill` and `--restart` without an address still cover every server

A third sets how a start opens its URL. `WWW_OPEN` holds the command, split at spaces, with `%s` where the URL goes, or the URL appended when no `%s` is given. `WWW_OPEN='brave-browser --app=%s'` opens a tree in an app window and `WWW_OPEN=firefox` in Firefox. It applies to loopback starts only; a share opens through `xdg-open`, as every start does without the variable.

A fourth holds a share's token. `WWW_SECRET` gives `--secret` a token of letters, digits, `-` and `_` instead of a random one. The token never goes on the command line, where `ps` shows it to every local user and the shell keeps it in its history, so `--secret=token` fails with `E22`. `--help` says whether the variable is set but never shows its value.

# Pages

URLs start with the served directory's path relative to home, eg `http://127.31.1.1:8080/scratch/github.com/unixfile/webstack/index.md`, so a URL names its tree. A directory outside home uses its absolute path.

Below the prefix, URLs mirror the file tree, so relative links in the docs work unchanged. A tree inside another served tree answers for its own paths.

A `.md` file renders as a page, a directory as a listing of its names, a text file as a highlighted code page with linkable lines, and anything else raw. Add `?raw` to any URL for the file itself.

A listing's first row names its views as commands: `ls` for names, `ls -l` for details and, in a repo, `<(git log)` for history, at the row's right end. The history alone is written as a process substitution, since it is a page of its own made from a command's output; a listing is the directory itself. The view on screen is not a link and stands in reverse video. The row stays on the history page, with `<(git log)` as the view on screen. `ls -l` opens `?details`. Each row there leads with the size and how long ago the file changed, eg `3d` or `5w`; hover shows the exact time. Directories show no size. A version shows no age, since git keeps no modification times. The view stays on through the directories it links, and `ls` returns to names.

A row over the details names the orders that sort them: `-S` over the sizes, `-t` over the ages and `-` over the names. A click sorts as `ls` does, largest or newest first. A second click on the same flag adds `r` before it, as in `-rt`, and reverses the order, and a third returns to name order. `-` returns to name order in one click; `ls` has no flag for that order, so the header is a dash with no flag after it. A click on it in name order reverses the names and it reads `-r`. The order in force stands in reverse video, and the view's command reads as typed, eg `ls -lrt`. Under `-S` directories come after the files, since they show no size. A version has no ages and so no `-t`. The sort stays on through the directories the view links. Up and Down pass over the sort row; the pointer or Tab reaches it.

## Missing pages

An absolute link such as `/x.md` points outside the prefix and gives 404. Its page names the trees nearest it, or every tree when none is near.

A path the tree lacks gives 404 and names the whole URL; the digits turn like a coin before they stop, with "Not Found" on the line below them. Below that, the page says where git saw the path go: the commit that deleted or moved it, with a link to its last version. It also names the files with names close to it in the nearest directory that exists.

## Links

Bare URLs in a doc become links.

A relative link in a doc whose target is missing gets a dotted red underline and names the missing path on hover. A link that climbs out of the tree is not checked, since another tree may answer there.

## Docs

Docs follow CommonMark with GitHub's extensions, rendered by goldmark: tables, strikethrough, task lists, bare URLs as links, footnotes and definition lists.

Lines join into a paragraph, and a blank line starts the next. A line that starts with a dash begins a list even right after text. Straight quotes and dashes turn typographic.

Text is hyphenated by the rules of its language, which the browser picks from a block's `lang`. www tells each block's language from its commonest words, eg `och`, `att` and `inte` for Swedish, and marks every block that is not English. It knows English, Swedish, Danish, Norwegian, German, Dutch, French, Spanish, Italian and Portuguese.

A block is a paragraph, heading, list, quote or table at the doc's top level. One too short to tell, such as a heading, takes the language of the doc as a whole. A doc in a language www does not know counts as English. Code and raw HTML are neither read nor marked.

A quote whose first line is only `[!NOTE]`, `[!TIP]`, `[!IMPORTANT]`, `[!WARNING]` or `[!CAUTION]` becomes an alert. The marker gives way to a small mono label that names its kind, and the alert keeps full strength where a plain quote fades. A marker with text after it on the same line stays a plain quote.

Wiki links work as relative links do. `[[name]]` opens `name.md` in the doc's directory and `[[sub/name]]` opens `sub/name.md`. A name with an extension, eg `[[main.go]]`, keeps it. `[[name|label]]` shows the label and `[[name#heading]]` opens at the heading.

A fenced block marked `dot` or `graphviz` becomes a diagram, drawn by Graphviz's `dot` when it is installed and kept by the hash of its source. Without `dot`, or when `dot` rejects the graph, the block shows as code under a line that says why. `dot` may not read image files the graph names.

Every gap between blocks is the same, whether the block is a paragraph, list, table or code. Headings sit close to their text.

## Linked from

Below a doc, linked from names each doc in the tree that links to it, by any relative spelling.

The list arrives after the page, since the server walks the tree for it: a few milliseconds in a repo, about 250 ms across all of `~/scratch`, 77,000 files. The server keeps each doc's links and reads a doc again only when its size or time changes. It reads a new tree's links in the background when the tree joins.

## Recent pages

The browser remembers the pages you open; the server keeps no such list. A search ranks the best five that match its words above other hits at their level. They come from the tree you are in, so one repo's reading never shows in another's. A version counts as its tree.

Each visit counts and recent visits weigh more, as in zoxide. A reload is no visit.

Each address is an origin of its own, so each visitor to a share sees only the pages that visitor opened. Online repos keep a list apart from the trees.

## Live reload

Open pages reload when what they show changes.

A doc takes its new version in place, without a reload. Each block that changed fills with the highlight colour and fades over three seconds, so the eye finds the change after a save.

A code page reloads when its file is written, a listing when a name in it changes, any page in a repo when its HEAD moves, and the index when a tree joins or leaves. A page whose file or tree is gone waits two seconds for it to return, then reloads to say so.

A page outlives its server. After three seconds without it, a line over the page says the server is gone and since when, and the page keeps trying each second. When a new server answers, eg after `make run` or `--restart`, the page takes its version from it and a line says new server, fading over three seconds. A restart takes about 60 ms, so its new server answers before the gone line shows.

Each page holds a server-sent events stream while its tab is visible. A hidden tab lets go, since a browser opens at most six connections to one host, and checks again when shown.

The server watches the directory of each open page and its repo's git directory through inotify, since editors save by renaming a new file over the old one. It polls every second where inotify fails.

## Look

The stylesheet is www's own. It began as md2html-default's, whose light palette it keeps, and the page colours sit as variables on `:root`.

A page turns dark when the OS asks for it, with a dark palette and syntax colours of its own. The dark palette is a first draft.

The header's right end names the mode in force, eg `▸ system`, and opens like the menu of versions. The menu sets the mode by hand: `light`, `system` and `dark`. Each shows in its own colours: light as a light page, dark as a dark one, system as the OS has it now. The one in force has a border. A choice, a click outside the menu or Escape closes it. `system` follows the OS again. The browser keeps the choice per server address, so a server on another address starts with the system's mode, and nothing goes to the server. The stylesheet writes each colour as `light-dark(light, dark)`, which browsers know since 2024; an older one shows pages without www's colours.

The browser loads the stylesheet once. Its URL names a hash of its content, eg `/_www/style.7b94a08e7731.css`, so it is kept for a year, and a changed stylesheet comes under a new URL.

The header names a tree by its base name and shows the full path on hover. Every tree page's breadcrumbs start at `index`, which links `/`.

The header, listings and www's own messages are mono; documents keep the serif. Mono text shows no ligatures, so `<=` stays two characters in any font.

Every page keeps one width, 58ch of the body font, and keeps space for a scrollbar, so the header holds still from page to page. Code is smaller than the body text and never under 11px. A code block in a document fits 99 columns in that width, and so does a diff, its leading sign included. A code page fits 104: a line number of up to four digits, a space and 99 columns of code. Longer lines scroll.

## Fonts

Pages load no fonts from the network. STIX Two Text, the serif, and Iosevka, the mono, come from the machine when installed. Otherwise they come from subsets built into www, which the browser caches for a year.

Each face has a core file for Latin text and www's own signs, about 25 KB for the serif and 15 KB for the mono, so a plain page fetches about 40 KB of font. A second file per face holds Greek, Cyrillic and symbols, and the browser fetches it only for a page that uses them.

Text shows at once in a fallback font and switches when the file arrives.

# Safety

www refuses to serve the home directory and every directory above it, such as `/home` and `/`. A tree exposes everything below it, and home holds `~/.ssh`, `~/.gnupg` and every token file.

## Symlinks

Symlinks inside the tree are followed, absolute ones included.

A link that leads out of the tree is followed only into a directory that holds a file named `.www-follow`, or below one, and only when other users could read the target. A file needs the others-read bit and a directory others-read and others-execute, checked from the marker's directory down. So nobody reads through www what `cat` would not show them.

A store of private files linked into repos, such as lord's `~/.local/share/lord`, opens with one marker in its root. Secrets kept there at mode 600 stay refused.

Opening a refused link shows the link, its target and the reason. The listing marks it refused, with the same on hover.

## Browsers

A loopback server answers only to loopback addresses, `localhost` and names that `/etc/hosts` maps to loopback.

Its Content-Security-Policy forbids every script but www's own, which it allows by hash. www's script reloads pages, marks changed blocks, walks listings by arrow and opens history folds. Raw HTML in a doc passes through, so a README in a cloned repo could otherwise run code on this origin.

## Code in a tree

www runs no code a tree supplies. It reads files and runs git with every option turned off that would start a program named in the repo's config: the pager, textconv, external diff, signature checks, fsmonitor and fetching.

## Other users

The addresses keep users out of each other's way, not out of each other's trees. Loopback is open to every local account, so on a machine shared with other users, they can reach your server.

Serve nothing there you would not show them.

# Search and controls

The box under the header searches what is below it as you type, ignoring case. It holds the keyboard, so typing on any page searches, wherever focus is.

## What a search finds

On a listing it searches the directory, nearest first. The directory's own names lead, then its own lines, at full strength. Each level below follows, dimmed, with names before lines. Pages you opened rise to the top of their level, and one outside the directory comes after every level, as `../other/x.md`. Paths are relative to the directory searched.

A line result takes one row, cut so the match shows, and opens a code page at its line. A file shows its first three matching lines, then `+N more`, which opens the file filtered by the query.

What git ignores comes last, behind one link: `1 hit in 1 ignored file`. Following it shows the hits. A repo inside the tree counts its own ignores. www asks git what it ignores at most once every 10 seconds per directory, so a file that becomes ignored can show as ordinary for that long. A search started inside an ignored directory treats its contents as ordinary. Limits apply per level, so a deep directory full of hits cannot crowd out a shallower one.

Any other page filters itself, and each match is marked:

- a code page keeps its matching lines with their numbers
- a doc keeps its matching blocks
- a history keeps its matching commits
- a syntax tree keeps the rows whose field, kind or text holds the query, with their folds opened

Enter jumps to the first match in the whole page. Below, after a gap and dimmed, follow the directory's results without the page's own file.

The query stays in the URL as `?q=`, so Back from a result or a reload shows the results again.

Hidden files are searched too, but not the stores machines write, such as `.git`, `.venv` or `.cache`, nor files over the code page limit. A search stops after half a second and says so. A share searches only what it serves.

## Keys

Escape clears the query and brings the page back. Escape on an empty query goes up, eg from a file to its directory or from a commit to its history. It highlights the page it left, so Enter goes back.

Up and Down move a highlight through a listing's entries, or through a search's matches and results, and Enter opens it. A match opens at its place in the whole page. The highlight survives a reload.

Left and Right move the cursor in the box and never navigate. With nothing to walk, Up, Down and Space scroll the page.

Shift+Tab in the box enters the header at the crumb Escape goes to, and Tab goes on from there to raw on a doc and Delete lock. Escape on a focused link or button returns to the box.

On a touch screen the box takes the keyboard only when tapped, since it would raise the on-screen keyboard.

## Commands

A colon as the box's first character lists the page's commands instead of searching. The list holds what the page offers: a listing's views and sorts, eg `ls -lt`, a code page's views, a doc's raw, the versions, then what every page has: up, index, the colour modes, copy, share, the bug button and the stop button. Each row names the command as the page does and says what it does.

What follows the colon filters the list: by the start of a name first, then anywhere in it, then by its letters in order, so `:-t` finds `ls -lt`, and last by what a command does. Up and Down move through the list, Enter runs the highlighted command and Escape returns to the page. A click runs a row too.

Kill server and Delete lock take a second Enter. Files and directories are not commands; search finds those.

## Copy buttons

Beside the box, ⧉ copies the page's text: what lies below the box, without www's own lines and links or line numbers. A code page copies as its file and a listing as its names. A doc copies its Markdown, and lines a range marks, eg `#L3-L26`, copy alone. What a search hides stays out.

Beside it, ⤴ copies a link to what is selected:

- selected or marked lines on a code page, pinned to the commit when the file is clean, eg `@261024.22/server.go#L10-L12`
- other selected text, as a link that scrolls to it and marks it, `#:~:text=`
- with nothing selected, the page

The link carries the page's own address, so a share's link opens on other machines and a loopback one only here. On the web address it is the forge's own link at the commit shown, eg `https://github.com/junegunn/fzf/blob/<commit>/main.go#L10-L12`, which opens in any browser.

## When controls show

The search row, box and buttons alike, sticks to the top of the window as the page scrolls. All three stay at hand while reading, and a jump to a line stops below the row and a code page's views.

The box, its buttons, raw, the stop button and a code page's views fade while nothing needs them, and keep their space. The search row then lets the text scroll through it. The crumbs, the header's status and the bug button stay.

They show while the page sits at its top, since the gaps they leave there look odd. Once the first line scrolls under the search row, they show only:

- while the query holds a character
- while the mouse moves over the column
- while keyboard focus sits on one of them or a crumb
- when the page scrolls up

Once shown, they fade only 2 s after the last of these ends, never at once.

# Servers and locks

Run `www` in a directory. It prints the tree's URL, opens the browser and writes a lockfile named after the endpoint, such as `127.31.1.1-8080.lock`. `cat` the lockfile to see the URL, root, PID and start time.

The URL is all it prints, so `url=$(www --no-open)` works. Running `www` again in a served directory prints the same URL and starts nothing.

Delete the lockfile to stop serving the tree. The Delete lock button on the tree's pages does the same.

## One server per address

Trees on one address share one server. The first `www` starts it in the background, and later ones hand their tree to it and return.

When something other than your www on that address holds the port, `www` fails at once with `E98` and names the listeners there.

The server exits when its last lock is gone. SIGTERM stops it at once and removes every lock but a persistent `.LOCK`, as does the Kill server button on the index at `/`.

Delete lock and Kill server answer 410 Gone. The page shows the code and "Gone" on the line below it, then names each tree stopped, its lock removed or kept, and whether the server serves on.

## Restarts

A running server keeps the binary it started with. `www --restart` moves every server's trees to the current binary, and `make install` and `make install-deb` run it for you. Open tabs reload onto the new binary, and it says how many did.

It moves only the servers its own binary started, so the package and a build from `make install` leave each other's servers alone. It names each server it leaves, and `-f` takes them too. `make install-deb` passes `-f`, since it removes the `make install` link.

A binary without the capability for port 80, such as the one from `make install`, leaves a port-80 server on its old binary and says so.

## The index

The index at `/` reads the same on every server. It lists each running server in the order of their addresses, with the address and the binary it runs. Each server's trees follow, with the lock's path on hover.

The server you are reading from says "this page". Only its trees carry a Delete lock button, since a server takes a stop only from its own pages. Another server's address links to its own index.

The package's binary and the one `make install` links are listed under No server when they run none, so both installs always show. A warning appears under a server whose binary changed on disk after it started, with the command that moves it to the new one.

With `--debug`, each server also says when its binary was built, from which commit and when the server started, and each tree when its lock was written. These serve debugging, so they are hidden otherwise.

To reach the server by name, add a line such as `127.31.1.1 www` to `/etc/hosts`. Type the name with its slash, eg `www/`, or the browser searches for it.

## Addresses

Each user owns 127.31.N.0/24 with N = uid − 1000. www serves on 127.31.N.1 by default, on port 80 where it may bind it, else 8080, and online repos on 127.31.N.2.

Uids from 1000 to 1255 fit; others need `--addr`. `-a` takes only loopback addresses; `--share` takes the others.

## Serving after reboot

`www -p` writes a lock in capitals, such as `127.31.1.1-8080.LOCK`. It holds no PID, and it outlives its server: Kill server, `--kill`, SIGTERM and a reboot all stop the server but leave the `.LOCK` in place.

The next server on that address serves the tree again. So does `www --restart`, which starts a server for every address whose persistent trees have none.

Deleting the `.LOCK`, or Delete lock on the tree's pages, ends the tree for good. `www -p` in a tree served by a plain `.lock` moves it to a `.LOCK`, and a plain `www` in a tree that holds a `.LOCK` keeps it.

## Systemd units

A systemd user unit runs `www --restart` at login. Both ship disabled. They serve from login to logout, and linger stays off.

- The package puts `www.service` in `/usr/lib/systemd/user/`. Enable it with `systemctl --user enable www`.
- `make install` puts `www-dev.service` in `~/.config/systemd/user/`. Enable it with `systemctl --user enable www-dev`.

Each brings back only the trees its own binary served last, so with both installs both units run.

The package also ships a system template, `www@.service`, which serves a user's trees from boot, before login, with the capability for port 80. Enable it with `sudo systemctl enable www@<user>`.

While the template or the package's user unit runs for a user, the other stands aside, since both run one binary on the same trees. `www-dev` runs beside either.

## Registry

Each endpoint has a registry directory such as `~/.local/state/www/127.31.1.1-8080/` with a symlink to each lock its server holds. `$XDG_STATE_HOME` replaces `~/.local/state` when set.

A joining `www` adds a link, and the server sees it through inotify, writes the lock and serves the tree. Where inotify fails the server polls every second. An flock on the directory keeps a tree from joining a server on its way out.

A lock left by a crash is detected on the next start through its PID and the kernel boot ID, then removed. A new server also clears what a crashed server left in the registry.

A lock also names the server's PID namespace. Inside a sandbox or container, another namespace's PID says nothing, so www takes that server to be running: it keeps the lock, lists the tree and refuses `--kill` and `--restart` with E1. If that server has crashed, its lock stays until a reboot or until you delete it. A lock written before locks named their namespace counts as the host's. A `www` that cannot read its own namespace from `/proc` treats every lock this way.

## A base path

`--base=PATH` puts every URL of a new server below PATH, eg `www --base=/www` serves `~/notes` at `/www/notes/`. It is for a proxy that gives www one part of a domain, eg `example.org/www/`, beside other sites. The proxy passes the path on whole, base included.

The base covers everything www writes and answers: trees, the index at `/www/`, the stylesheet and fonts, the stop and bug forms, redirects and the web address's repos. `/www` alone redirects to `/www/`. A path outside the base is a 404.

A base is one or more segments of letters, digits, `-`, `.` and `_`, and no segment starts with `_`.

A base belongs to a server, not to a tree. A tree that joins a running server takes the server's base, and www says so when `--base` named another. `--restart` keeps each server's base, so it takes no `--base`; to change one, stop the server with `--kill` and start it again.

# Sharing

`www --share [DIR]` serves a tree on the address of the default route, so other machines on the LAN can read it. www asks the kernel which source address it would use to leave the machine, and the printed URL shows it. Over a full-tunnel VPN that is the tunnel's address.

`WWW_SHARE`, eg `WWW_SHARE=mate.peer.home`, gives a bare `--share` another address, for `--kill --share` and `--restart --share` as well.

`--share=HOST` names the address instead, an IP or IP:PORT, joined with `=`: `--share 192.168.1.5` would read the address as the directory. Any IPv4 address the machine lets www bind will do, `0.0.0.0` included. A name will do too, such as `--share=mate.peer.home`: www binds the address it resolves to, which an interface here must hold.

The printed URL carries the address's name when its reverse lookup resolves back to it, so a share on a WireGuard address with its own DNS prints `http://mate.peer.home/notes/`. Otherwise it carries the address. The port defaults to 80 where www may bind it, else 8080, as on loopback. `-a` takes only loopback addresses, so a typo there cannot share a tree.

A shared address gets a server of its own, apart from the loopback one. More trees join it with the same `--share` address. The lock is named after the address, such as `192.168.1.5-80.lock`, and deleting it stops the share.

`www --kill --share` stops every tree on that address. `www --restart --share` moves the trees to a new binary. `www -l` lists shared trees with the rest.

On `0.0.0.0` the printed URL carries the machine's name, since the server answers on every address. `0.0.0.0` claims its port on every address, so it cannot bind a port that any listener holds, the loopback www included. www then fails with `E98` and names the listeners on that port. Share on one address, or give `0.0.0.0` another port.

## What a visitor sees

A shared tree mounts at its base name, eg `http://192.168.1.69/notes/`. The URL does not reveal where the tree lives on the machine.

The server serves only what other users on the machine could read. A file needs the others-read bit, and every directory from the tree's root down to it needs others-read and others-execute. Links out of the tree follow the `.www-follow` rule as on loopback.

Names that begin with a dot are neither listed nor served, at any depth, nor are links that lead to them inside the tree: `.git` holds the history and every deleted file.

History pages show commit messages alone, without trailers, and only when other users could read the repo.

Everything else in the tree is served, so share a directory that holds only what you mean to share.

A refused entry shows as refused, without the reason, since the reason names paths outside the tree. The index at `/` lists the shared trees by name. It shows no lock paths, no other addresses and no buttons. No page has a Delete lock or Kill server button, so no visitor can stop the server.

The server answers to any `Host` header, since visitors use whatever name reaches the machine. The Content-Security-Policy forbids every script but live reload's.

## Nobody can connect

Every share start prints a note on stderr after the URL, with the capture that tells where a connection dies:

```
note: other machines connect to 192.168.1.69:80; a firewall here may drop them unseen.
      Check with: sudo tcpdump -ni wlp0s20f3 tcp port 80
```

www cannot find out by itself. A firewall drops the connection before any socket sees it, reading the ruleset needs root, and a test from this machine goes through loopback and passes.

Run the capture and load the URL on the other machine:

- Nothing arrives: the request never reaches this machine. The other machine is on another network, a guest network or mobile data, or the router keeps wifi clients apart
- SYNs arrive and none is answered: a firewall here drops them. `sudo nft list ruleset` shows it; look for `policy drop` on an input chain with no accept for the port
- SYNs are answered with RST: nothing listens there. Check the address in the printed URL

To open the port to the LAN until the firewall reloads:

```
sudo nft insert rule inet <table> input ip saddr 192.168.1.0/24 tcp dport 80 accept
```

`<table>` is the table that drops, such as `filter` or `laptop`.

## Secret paths

`--secret` mounts the tree under a random 128-bit token instead of its name, eg `http://192.168.1.5/lz7sjx3g2cvx54ev7ndbzf7i7q/`, and leaves it off the index. Only those holding the link reach it. `WWW_SECRET` sets a token of your own, of letters, digits, `-` and `_`. The command line takes none, since `ps` would show it to every local user.

The token also guards against DNS rebinding. Without it, a hostile web page could read a share through the browser of a visitor on the same network.

The token lies in the tree's lockfile and in the private registry. `--restart` keeps it, and passes it to the new server on the command line, where other local users can see it in `ps`.

# Speed

www is fast. A page from a running server takes one to three milliseconds and a server starts in about 40 ms. A test in `make test` fails any page that takes over 5 ms.

## Git's answers

A server keeps git's answers, so most pages run no git at all. It keeps which repo each directory lies in, the histories pages show and the commits they open. Commits never change and are kept by their full hash. `--cache` sets the memory for all of it.

The server reads HEAD from the repo's files, and a history is kept under the commit HEAD names. A new commit misses the cache, so nothing shown goes stale. A `.git` that appears or goes makes the server ask git again.

The cache holds only what someone has visited, with one exception. When a tree joins, the server asks git in the background for what its listing and its README show, since those open first. The log says how long that took.

## Slow histories

In a large repo git can take a second for a history, because it reads every commit back to the ones that touched the path.

When it takes over 100 ms, the history page says so and names `git commit-graph write --reachable --changed-paths`. That builds git's index of the paths each commit changed and makes such histories several times faster. www never writes to the repo itself.

## Benchmarks

`make bench` times what a reader waits for: a search across a generated tree of 4,000 files and 24 MB, and a code page's first view per language. Each language is timed three ways: lexing alone, the whole page, and the page with scope boxes.

`BENCH` picks benchmarks by pattern and `COUNT` repeats them, eg `make bench BENCH=Search COUNT=10` for benchstat.

Python, JavaScript and shell are timed on argparse.py, jquery.js and bash_completion from Debian's packages, and skipped where those are not installed.

Warm pages have no benchmark, since `TestPageBudget` in `make test` holds them under 5 ms. `make test` runs it in a step of its own after the other tests, so nothing compiles or runs beside the measurement.

# Testing

Two kinds of test, split by what a rewrite in another language would keep.

## Go tests

They check internals: parsers, edit distance, path math, the lexers against chroma. They also drive the handler in process, which is fast and covers most behavior. A rewrite deletes them.

Write new Go tests as tables, with the cases in `testdata/` where they fit. Purpose: a later move to black-box form then reuses the data.

## Black-box tests

They run the built binary and check what a user sees. They cover what in-process tests cannot reach: flag parsing, the listener, the public executable, process lifecycle and browser pages. The first case is a missing path on `--public`, which shows the 404 coin.

The suite is pytest in `tests/`, with its own `pyproject.toml`. `make blackbox` builds the binary and runs it. HTTP, browser and process cases exist so far; the CLI kind below comes with its first case.

- CLI: `subprocess` checks stdout, stderr and the exit code apart, including `E{code}` messages
- HTTP: `httpx` against a server the suite starts on a free port
- Browser: Playwright in `tests/test_browser.py`, asserting DOM, layout and computed style, never pixels. It uses Playwright's Chromium where installed, else the machine's Brave, and skips with the reason when no browser starts, eg in a sandbox. Playwright drives only its own Firefox build, so a Firefox check stays a script in `csi/sketch/browser-tests/`
- Processes: `subprocess.Popen` with signals and waits for locks, restarts and the journal, in `tests/test_processes.py`. A case that needs a new PID namespace skips with the reason where `unshare` is refused, eg in a sandbox

Rules for the suite:

- It reads the reader's binary from `TEST_BIN` and the public one from `TEST_PUBLIC_BIN`, and assumes nothing else about them
- An HTTP or CLI case is a TOML file in `tests/cases/`: argv with expected stdout and exit code, or a request with the expected page. A parametrized test loads it
- A case names the server forms it runs on: `public`, the public executable, or `default`, a reader's server, as wwwvm's unit runs it
- A case compares the page without its script and style, since a reader never sees their words
- `tests/harness.py` seeds the cache on disk, since a public server clones only over https from the open internet. It is the one place the suite knows the cache's layout
- Name each case after the decision it checks
- Make runs repeatable: git repos built with fixed dates, a throwaway `HOME` and `XDG_*`, a free port, no network
- Replace volatile parts in expected output before comparing: stardates, timestamps, ports, temp paths

## Race detector

Run `go test -race` when a change touches locks, restart, serve or the watcher.

## Heavy runs

Corpus crawl, differential lexing, fuzzing, mutation testing and soak runs go on the user's own machine. The model writes the script and its instructions in `csi/sketch/`. The user runs it and names the output path.

## Later

The large Go tests, starting with `web_test.go`, move to black-box form when a rewrite is decided. The old binary then serves as the check on the new cases, and the same suite runs against both.

