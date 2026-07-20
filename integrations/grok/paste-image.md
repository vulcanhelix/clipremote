# Paste image from laptop clipboard (clipremote)

You are running on a remote host. The user's laptop clipboard is bridged by **clipremote**.

## Instructions

1. Run this shell command:

```bash
clipremote paste --clipboard 2>/dev/null || clipremote latest
```

2. The command prints a filesystem path to a PNG (usually `~/.cache/clipremote/latest.png`).
3. Use the **Read** tool on that path so you can see the image.
4. If both fail, tell the user:
   - On the Mac: ensure `clipremote daemon` is running and they connected with `clipremote ssh <host>`.
   - Or attach manually: `@~/.cache/clipremote/latest.png`

## Notes

- Prefer the printed path from `clipremote paste` / `latest`.
- Do not ask the user to scp files.
