# Bundled sound clips

Embedded into the binary (`//go:embed sounds/*.mp3`, see `internal/app/sound.go`)
and played per-event. Each is the default for its event; override any via the
matching `[sound]` path in `config.toml`.

| File | Event |
|------|-------|
| `mark_quest_as_done.mp3` | a quest marked done |
| `mark_objective_as_done.mp3` | an objective checked off |
| `mark_quest_as_active.mp3` | a quest taken up (active) |
| `enter_tavern.mp3` | entering the Tavern (also on launch) |
| `enter_wilds.mp3` | setting out to the Wilds |
| `add_connection_to_quest.mp3` | a Jira/PR/rune/agent linked to a quest |
| `open_npc_selection.mp3` | the agent picker opens |

Provided by the project owner. Replace any file (same name) or point its
`[sound]` config key at your own clip to customize.
