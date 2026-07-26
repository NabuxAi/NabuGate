# Skills

Agent skills for working with NabuGate. Drop one into `~/.claude/skills/` (or a
project's `.claude/skills/`) and the agent picks it up automatically.

```bash
cp -r skills/nabugate ~/.claude/skills/
```

| skill | when it triggers |
|---|---|
| [`nabugate`](./nabugate/SKILL.md) | any project that needs an LLM, embeddings, images or TTS; minting a per-app token; restricting a key by origin; debugging routing, empty responses, embedding widths or a Coolify deploy that exits with no logs |

## Why this lives here

The skill is the answer to "how does my project talk to NabuGate?" — so it
belongs next to the gateway it describes, not in one person's home directory.
Anyone can be pointed at this folder.

Keep it current with the gateway. Every rule in it came from something breaking
in production; if a new failure teaches a rule, write it down here rather than
in a commit message nobody will find again.
