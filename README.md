# clipremote

**Auto-sync screenshots from your laptop into a remote Linux host** so Grok, Claude Code, Codex, and other agents can see them over SSH — no `scp` every time.

```
 Laptop (macOS)                              Remote (Linux)
┌──────────────────────────┐                ┌─────────────────────────────┐
│ Screenshots folder       │                │                             │
│ (Desktop / Screenshots)  │   SSH push     │  ~/.cache/clipremote/       │
│         │                │ ─────────────► │    latest.png  ← always use │
│         ▼                │  (mux)         │    history/     (max 20)    │
│  clipremote daemon       │                │                             │
│  (LaunchAgent / login)   │                │  Grok / Claude / Codex      │
└──────────────────────────┘                └─────────────────────────────┘
```

In the remote agent, attach:

```text
@~/.cache/clipremote/latest.png
```

That path always points at the **newest** synced image.

---

## Vibecoders guide

You SSH into a box. You talk to Grok/Claude there. You want it to **see your screenshots** without learning `scp`.

### The only path that matters

```text
@~/.cache/clipremote/latest.png
```

Paste that in the agent. Always. New screenshot → that file updates. Done.

### Setup once (copy-paste)

Pick version from [Releases](https://github.com/vulcanhelix/clipremote/releases/latest) (example: `0.1.6`).

**1. On the remote box** (where the agent runs):

```bash
V=0.1.6   # bump if a newer release exists
mkdir -p ~/.local/bin
curl -fsSL -o ~/.local/bin/clipremote \
  "https://github.com/vulcanhelix/clipremote/releases/download/v${V}/clipremote_${V}_linux_amd64"
chmod +x ~/.local/bin/clipremote
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc
clipremote setup --remote
```

(ARM server? `linux_arm64` instead of `linux_amd64`.)

**2. On your Mac:**

```bash
V=0.1.6
# M1/M2/M3 → darwin_arm64 | Intel → darwin_amd64
ARCH=darwin_amd64
curl -fsSL -o /usr/local/bin/clipremote \
  "https://github.com/vulcanhelix/clipremote/releases/download/v${V}/clipremote_${V}_${ARCH}"
chmod +x /usr/local/bin/clipremote

clipremote setup
clipremote host add box YOU@YOUR_SERVER   # same as: ssh YOU@YOUR_SERVER
clipremote install-service                # survives reboot
```

**3. Tell your agent once** so you never paste the path again.

Drop this into the **remote** machine (where Grok/Claude runs), not your laptop:

- Claude: `~/.claude/CLAUDE.md` or `~/.claude/rules/clipremote.md`
- Grok: `~/.grok/rules/clipremote.md`
- Or any project `AGENTS.md` / `CLAUDE.md`

Copy-paste this block:

```markdown
## Screenshots (clipremote)

The user's latest laptop screenshot is **always** at:
`~/.cache/clipremote/latest.png`

Older shots (if needed): `~/.cache/clipremote/history/`

When they say "screenshot", "look at this", "see my screen", or share UI feedback:
1. Read `~/.cache/clipremote/latest.png` yourself.
2. Do **not** ask them to paste the path, scp, or send a Mac `/Users/...` file.
```

Ready-made file: [`integrations/agent-rules/clipremote.md`](integrations/agent-rules/clipremote.md)

```bash
# on the remote box
mkdir -p ~/.claude/rules ~/.grok/rules
curl -fsSL -o ~/.claude/rules/clipremote.md \
  https://raw.githubusercontent.com/vulcanhelix/clipremote/main/integrations/agent-rules/clipremote.md
cp ~/.claude/rules/clipremote.md ~/.grok/rules/clipremote.md
```

### Every day after that

1. Screenshot (save to Desktop / CleanShot — whatever lands a PNG on Desktop).
2. Wait one second.
3. In the agent just say **“look at my screenshot”** / **“see this UI”** — no path paste.  
   (They already know to open `~/.cache/clipremote/latest.png`.)

### If it breaks

| Vibe check | Do this |
|------------|---------|
| Agent can’t see the image | New screenshot, wait 1s, try `@~/.cache/clipremote/latest.png` again |
| Nothing uploads | On Mac: `tail -20 ~/Library/Logs/clipremote.log` |
| “Permission denied” | Your SSH key needs to work without typing a password |
| Wrong Mac CPU binary | `bad CPU type` → use the other `darwin_*` build |

That’s it. The long docs below are for when the vibe fails.

---

## Why this exists

Coding agents can read images, but when the agent runs **on a server over SSH**, your screenshot lives on the **laptop**. Terminal clipboard bridges (OSC 52, `grok wrap`, etc.) help copy **text out** — they do not get **images in**.

`clipremote` watches a local screenshots folder, pushes new images over SSH, and keeps a fixed remote path plus a short history.

---

## Requirements

| Side | Needs |
|------|--------|
| **Laptop** | macOS (primary), SSH key access to the remote host |
| **Remote** | Linux, `clipremote` binary on `PATH` (e.g. `~/.local/bin`) |
| **Auth** | Passwordless SSH (key + agent / Keychain) so the daemon can push without prompts |

Linux laptops work for `push` / folder mode; the polished login service path is documented for macOS first.

---

## Install

Replace `vX.Y.Z` with the [latest release](https://github.com/vulcanhelix/clipremote/releases/latest) tag (e.g. `v0.1.6`).

### 1. Remote Linux host (once)

```bash
# Pick the asset for your CPU: linux_amd64 or linux_arm64
mkdir -p ~/.local/bin
curl -fsSL -o ~/.local/bin/clipremote \
  "https://github.com/vulcanhelix/clipremote/releases/download/vX.Y.Z/clipremote_X.Y.Z_linux_amd64"
chmod +x ~/.local/bin/clipremote
export PATH="$HOME/.local/bin:$PATH"

clipremote setup --remote
clipremote doctor
```

Ensure non-interactive SSH can find the binary (`~/.local/bin` is tried first by the client).

### 2. Mac laptop (once)

```bash
# Apple Silicon → darwin_arm64 | Intel → darwin_amd64
curl -fsSL -o /usr/local/bin/clipremote \
  "https://github.com/vulcanhelix/clipremote/releases/download/vX.Y.Z/clipremote_X.Y.Z_darwin_amd64"
chmod +x /usr/local/bin/clipremote

clipremote setup
# Point at your screenshots folder if auto-detect is wrong (common: Desktop)
# Edit ~/.config/clipremote/config.toml → screenshots_dir = "/Users/YOU/Desktop"

clipremote host add myserver you@your-host.example
clipremote install-service    # starts at login, restarts if it dies
```

Confirm:

```bash
clipremote host list
launchctl print gui/$(id -u)/com.clipremote.daemon | head -20
tail -20 ~/Library/Logs/clipremote.log
```

You should see `state = running`, your screenshots folder, and `ssh mux ready` for the configured host.

### From source

```bash
git clone https://github.com/vulcanhelix/clipremote.git
cd clipremote
./scripts/build.sh
# binaries in dist/
./scripts/install.sh           # this machine as laptop-side
./scripts/install.sh --remote  # this machine as server-side
```

---

## Daily use

1. Take a screenshot that **saves a file** into the watched folder (Desktop, CleanShot default, etc.).
2. Within about a second the daemon uploads it.
3. On the remote host, in Grok / Claude / Codex:

```text
@~/.cache/clipremote/latest.png
```

No need to open `clipremote ssh` for each shot once the daemon and SSH keys are set up.

### Manual push (optional)

```bash
clipremote push myserver
# clipremote push --dir ~/Desktop -n 20 myserver
```

### With `grok wrap` (text copy-out + images)

```bash
grok wrap clipremote ssh myserver
# or: clipremote ssh myserver   then run grok on the remote
```

`grok wrap` helps **copy text from** the remote TUI to your laptop clipboard. `clipremote` handles **images to** the remote.

---

## Teach your agents (recommended)

So every new session knows to read the stable path without asking for `scp`:

### Grok

Copy the rule into the host’s Grok rules (or your project):

- Example rule: [`integrations/agent-rules/clipremote.md`](integrations/agent-rules/clipremote.md)
- Install on the **remote** machine, e.g. `~/.grok/rules/clipremote.md`

### Claude Code

- Same rule file → `~/.claude/rules/clipremote.md` on the remote
- Optional slash command: [`integrations/claude/paste-image.md`](integrations/claude/paste-image.md) → `~/.claude/commands/paste-image.md`

### Any agent

Instruct it:

> Latest user screenshot is always at `~/.cache/clipremote/latest.png`. Read that file when the user refers to a screenshot or UI. Do not ask for laptop paths or scp.

---

## Configuration

Laptop: `~/.config/clipremote/config.toml`

```toml
port = 18765
auto_push = true
history = 20                 # how many images the *remote* keeps (on ingest)
control_path = "~/.ssh/clipremote-%r@%h:%p"
screenshots_dir = ""         # empty = auto-detect Desktop / Pictures/Screenshots / …
screenshots_n = 20           # how many recent local files to consider on `push`
source = "folder"            # folder | clipboard | auto

[[hosts]]
name = "myserver"
ssh = "you@your-host.example"
```

| Key | Meaning |
|-----|---------|
| `screenshots_dir` | Folder to watch (set explicitly if auto-detect is wrong) |
| `screenshots_n` | Local “recent files” window for `push` / seed |
| `history` | Applied on the **remote** when ingesting — drop oldest beyond N |
| `source` | `folder` (default, best for CleanShot), `clipboard`, or `auto` |
| `auto_push` | Daemon pushes new files to configured hosts |
| `[[hosts]]` | Named targets for auto-push and `clipremote push` / `ssh` |

Remote uses the same config file shape; set `history = 20` there (or run `clipremote setup --remote`).

---

## Commands

| Command | Side | Purpose |
|---------|------|---------|
| `setup` | both | Config dirs, defaults; macOS writes LaunchAgent plist |
| `install-service` | laptop | Install + start login daemon (survives reboot) |
| `daemon` | laptop | Run watcher in foreground |
| `host add\|list\|rm` | laptop | Manage remote targets |
| `push [name]` | laptop | Upload recent folder images (or `--clipboard`) |
| `ssh <target>` | laptop | SSH with ControlMaster + reverse tunnel |
| `ingest` | remote | Read image from stdin → `latest.png` + history |
| `latest` | remote | Print path to `latest.png` |
| `paste` | remote | Pull from reverse-tunneled laptop daemon |
| `doctor` | both | Diagnose daemon, hosts, last ingest, PATH |
| `version` | both | Print version |

---

## How it works

1. **Daemon** (laptop) polls the screenshots folder (~1s).
2. On a **new** image file, it SSHs to each configured host and runs `clipremote ingest` with the file on stdin.
3. **Ingest** (remote) writes `~/.cache/clipremote/latest.png` and a timestamped file under `history/`, pruning to `history` entries.
4. Agents **read** `latest.png` (or older history files if needed).

SSH uses a **ControlMaster** mux so pushes are fast and do not require an interactive session. Prefer keys loaded in the agent (on macOS, Keychain / `ssh-add --apple-use-keychain`).

Optional **clipboard** mode and reverse-tunnel `paste` remain available; **folder watch is the recommended path**.

---

## Troubleshooting

| Symptom | What to try |
|---------|-------------|
| New screenshots not appearing remotely | `tail -f ~/Library/Logs/clipremote.log` on the laptop; confirm daemon `state = running` |
| `auto-push: no hosts configured` | `clipremote host add NAME user@host` |
| `clipremote: cannot execute: Is a directory` | Remote `PATH` is hitting a repo folder named `clipremote`; install the **binary** to `~/.local/bin/clipremote` |
| `Permission denied` on push | Fix SSH keys; test `ssh user@host true` with no password |
| Wrong folder watched | Set `screenshots_dir` in config to the absolute path CleanShot/macOS uses |
| `latest.png` is an old/small crop | A newer file may not have landed yet, or a tiny CleanShot region was newest by mtime — take another full shot |
| Agent asks for `/Users/...` paths | Install the agent rule on the **remote** (see above) |
| After reboot, no pushes | Confirm LaunchAgent: `clipremote install-service`; ensure SSH key is available at login |

```bash
# Laptop
clipremote doctor
clipremote host list

# Remote
clipremote doctor
ls -la ~/.cache/clipremote/latest.png
```

---

## Security

- Laptop daemon listens on **`127.0.0.1` only** (pull mode).
- Uploads use **your SSH credentials** only.
- Images are stored under the remote user’s home cache; history is capped.
- Do not expose the daemon port to the public internet.

---

## Related projects

- [claude-ssh-image-skill / ccimg](https://github.com/AlexZeitler/claude-ssh-image-skill) — pull + skill  
- [cc-clip](https://github.com/ShunmeiCho/cc-clip) — reverse tunnel + clipboard shims  

`clipremote` focuses on **push-from-folder**, a **stable remote path**, and **login daemon** auto-sync.

---

## Development

```bash
go test ./...
./scripts/build.sh
# dist/clipremote_<version>_<os>_<arch>
```

## License

MIT
