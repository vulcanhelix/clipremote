# Integrations

Drop these onto the **remote** machine (where Grok / Claude / Codex run), not the laptop.

| File | Install as |
|------|------------|
| [`agent-rules/clipremote.md`](agent-rules/clipremote.md) | `~/.grok/rules/clipremote.md` and/or `~/.claude/rules/clipremote.md` (or project `AGENTS.md` / `CLAUDE.md` snippet) |
| [`claude/paste-image.md`](claude/paste-image.md) | `~/.claude/commands/paste-image.md` |
| [`grok/paste-image.md`](grok/paste-image.md) | Grok skill or slash-command location for your install |

Agents only need the stable path:

```text
@~/.cache/clipremote/latest.png
```
