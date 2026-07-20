# Changelog

## Unreleased

- Documentation overhaul for general users (not machine-specific)
- Generic agent integration rules under `integrations/`

## v0.1.6

- Default screenshot history and `screenshots_n` raised to **20**

## v0.1.5

- `clipremote install-service` — login LaunchAgent (survives reboot)

## v0.1.4

- Folder watch auto-push to configured hosts (no open SSH session required)
- SSH ControlMaster warm-up for configured hosts
- Remote history cap defaults

## v0.1.3

- Remote exec uses absolute binary paths (avoid repo directory named `clipremote`)

## v0.1.2

- Prefer screenshots **folder** sync over clipboard
- `push --dir` / `screenshots_dir` / `source = folder`

## v0.1.1

- Improved macOS clipboard read (AppKit / pngpaste / file URLs)

## v0.1.0

- Initial release: ingest, daemon, push, ssh wrap, doctor
