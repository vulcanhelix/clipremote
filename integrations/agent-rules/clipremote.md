# clipremote — remote screenshots

This host receives screenshots from the user's laptop via **clipremote** (folder auto-sync over SSH).

## When the user shares a screenshot, UI, or "look at this"

1. **Read** `~/.cache/clipremote/latest.png`.
2. Do **not** ask them to `scp`, paste laptop paths (`/Users/...`, `C:\...`), or re-upload.
3. If the file is missing or clearly stale vs what they describe, say so and ask them to capture a new screenshot into their watched folder (daemon syncs within ~1s), then re-read `latest.png`.

## Facts

- Latest image: `~/.cache/clipremote/latest.png`
- History (capped, default 20): `~/.cache/clipremote/history/`
- Project: https://github.com/vulcanhelix/clipremote

## Do not

- Do not invent or use paths on the user's laptop filesystem.
- Do not require manual transfer for routine screenshots when clipremote is set up.
