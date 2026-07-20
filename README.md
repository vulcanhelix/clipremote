# clipremote

**Paste screenshots from your Mac into remote Grok / Claude / Codex over SSH** — without `scp` every time.

```
Mac clipboard (CleanShot / Copy Image)
        │
        ▼
 clipremote daemon  ──ssh push──►  ~/.cache/clipremote/latest.png
        │                              + remote clipboard (best effort)
        │
        └── clipremote ssh host   (ControlMaster + reverse tunnel)
```

Then in remote Grok: **`Ctrl+V`**, or always:

```text
@~/.cache/clipremote/latest.png
```

## Why this exists

Agent TUIs are multimodal, but over SSH the **image lives on your laptop** while the **process runs on the server**.  
`grok wrap ssh` fixes **copy text out** (OSC 52). It does **not** bridge images **in**.

`clipremote` does.

## Install

### From source

```bash
git clone https://github.com/vulcanhelix/clipremote
cd clipremote
./scripts/build.sh
# put dist/clipremote on PATH (both Mac and remote)
```

### Install script

```bash
# on either machine (builds if go.mod present, else downloads release)
./scripts/install.sh           # local Mac
./scripts/install.sh --remote  # Linux host
```

Cross-compile all targets:

```bash
./scripts/build.sh
# dist/clipremote_0.1.0_darwin_arm64
# dist/clipremote_0.1.0_linux_amd64
# ...
```

Copy the Linux binary to the server:

```bash
scp dist/clipremote_0.1.0_linux_amd64 user@host:~/.local/bin/clipremote
ssh user@host 'chmod +x ~/.local/bin/clipremote && clipremote setup --remote'
```

## Quick start

### 1. Mac (once)

```bash
clipremote setup
clipremote daemon &       # or launchctl load the plist from setup
clipremote host add box user@your-server
```

Default mode watches your **screenshots folder** (Desktop / Pictures/Screenshots) — CleanShot-friendly, no clipboard needed.

### 2. Remote Linux (once)

```bash
clipremote setup --remote
```

### 3. Every day

```bash
clipremote ssh box
# take a screenshot → auto-uploads within ~1s
# in remote Grok:
@~/.cache/clipremote/latest.png
```

Force upload of the last 10 local screenshots:

```bash
clipremote push box
# clipremote push -n 10 --dir ~/Desktop box
```

Pull on demand (uses SSH reverse tunnel from `clipremote ssh`):

```bash
# on remote
clipremote paste
```

## Commands

| Command | Where | What |
|---|---|---|
| `daemon` | Mac | Watch clipboard + HTTP pull server (`127.0.0.1:18765`) |
| `ssh <host>` | Mac | SSH with ControlMaster, mark host active, `-R` pull tunnel |
| `push [host]` | Mac | Push current clipboard image now |
| `host add\|list\|rm` | Mac | Manage hosts in `~/.config/clipremote/config.toml` |
| `ingest` | Remote | Stdin image → `latest.png` (+ optional `--clipboard`) |
| `paste` | Remote | Fetch from reverse-tunneled daemon → ingest |
| `latest` | Remote | Print path to `latest.png` |
| `doctor` | Both | Diagnose daemon / tunnel / clipboard / last ingest |
| `setup` | Both | Write config, dirs, launchd / PATH tips |
| `xvfb` | Remote | Start virtual display for headless clipboard |

## True paste vs stable path

| Mode | Works when | How |
|---|---|---|
| **`Ctrl+V` in Grok** | Remote has a clipboard Grok can read (GUI session, or Xvfb + `DISPLAY`) | Auto-push sets remote clipboard |
| **`@~/.cache/clipremote/latest.png`** | Always (headless VPS fine) | File updated on every push/paste |

Honest default for most SSH servers: use the **stable path** (zero friction, no display). Turn on Xvfb when you want real clipboard paste.

## Config

`~/.config/clipremote/config.toml`:

```toml
port = 18765
auto_push = true
history = 20
control_path = "~/.ssh/clipremote-%r@%h:%p"
screenshots_dir = ""          # empty = auto (Desktop, Pictures/Screenshots, …)
screenshots_n = 10            # last N local images to sync on `push`
source = "folder"             # folder | clipboard | auto

[[hosts]]
name = "box"
ssh = "user@hostname"
```

## Security

- Local daemon binds **`127.0.0.1` only**.
- Reverse forward is remote-localhost → your Mac loopback (OpenSSH default).
- Push uses your existing SSH auth (keys / ControlMaster).
- Images land in your home cache dir; history capped (default 20).

## Grok notes

- Prefer: `grok wrap clipremote ssh host` if you also want OSC 52 **text copy-out**.
- Optional skill: copy [`integrations/grok/paste-image.md`](integrations/grok/paste-image.md) into your Grok skills/commands so `/paste-image` pulls and reads the file.
- CleanShot: use **copy to clipboard**, not only “save to Desktop”, for auto-push. (Desktop folder watch can be added later.)

## Related tools

- [ccimg](https://github.com/AlexZeitler/claude-ssh-image-skill) — pull + Claude skill  
- [cc-clip](https://github.com/ShunmeiCho/cc-clip) — reverse tunnel + xclip shim / Xvfb  

`clipremote` is **push-first** (clipboard updates before you paste), tool-agnostic, and always maintains a stable file path.

## Development

```bash
go test ./...
./scripts/build.sh
```

## License

MIT
