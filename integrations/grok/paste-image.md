# clipremote /paste-image

Read the latest screenshot synced from the user's laptop via clipremote.

## Instructions

1. Read `~/.cache/clipremote/latest.png` with the image/Read tool.
2. Act on the user's request about that image.
3. If missing or stale: ask them to capture a new screenshot into their watched folder (daemon auto-syncs in ~1s), then re-read.
4. Do not ask for `scp` or laptop paths like `/Users/...`.

Optional shell check:

```bash
clipremote latest
ls -la ~/.cache/clipremote/latest.png
```
