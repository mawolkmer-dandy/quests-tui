# UI consistency — apply changes to ALL similar surfaces

**The rule:** when the user asks for a change to something they see (a hint's
wording, an inline emblem, a scroll behaviour, a cursor affordance…), apply it
to **every** place that shows the same kind of thing — not just the one screen
they happened to point at. The user reports the instance they noticed; the
expectation is that it lands everywhere that instance's pattern appears.

If applying it everywhere is awkward because the logic is duplicated across
surfaces, **that duplication is the bug** — rework it into a shared component
and route every surface through it, so the next change lands in one place.

Only ask the user "should this also apply to X?" when X is genuinely a
different case (e.g. a read-only archive view vs. an editable list). Don't ask
about obviously-parallel surfaces — just apply it.

## The surfaces that render the same things

These all render the same list/row concepts and must stay in lockstep:

| Surface | Entry point |
|---|---|
| Single-column outline / Wilds | `renderOutlineRowLine` (app.go) |
| Two-column Tavern — campaigns column | `renderOutlineRowLine` (via `wrapItems` withSpans) |
| Two-column Tavern — rail boxes | `renderBoxItemLine` (tavern_columns.go) |
| Campaign detail — quest list | `renderFocusListRow` (modals.go) |
| Section detail pages | `renderFocusListRow` (modals.go) |
| Quest detail — Sigils pane | `focusCodeLines` (sync.go) |

## Shared components (change these once → everywhere follows)

- **Row title content** → `rowTitleView(row, isCursor)` (app.go). The editor
  when focused, else the kind-specific content (quest/rune/lookout/track).
- **Quest title width** → `questTitleView` (app.go). Reserves a trailing caret
  cell so the emblems/progress after a quest title never shift when the edit
  caret sits at the end of the name.
- **Inline connection emblems** → `withConnectionIcons(line, row)` (app.go) /
  `connectionIcons(q)` (connections.go).
- **Action hints** → `actionHintParts` + `keyHint`/`joinHints` (app.go),
  rendered on the bottom **status line** (`statusHint`/`statusBar`), never
  inline. Detail-page equivalent: `sigilStatusLine` (links.go).
- **Row line assembly** → `ui.RenderRow` (ui/outline.go).
- **Scrolling** → wheel scrolls the VIEWPORT only, never the cursor; each render
  re-centres on the cursor only when `cursorMoved` (a keyboard move). See
  `handleWheel`/`handleFocusWheel` and the `cursorMoved` gating in every render.

## Checklist before finishing a UI change

1. List every surface above that shows the thing you changed.
2. Route them through the shared component (or create one if you're duplicating).
3. If you deliberately skip a surface, say so and why.
4. Add/extend a test that asserts the behaviour on more than one surface
   (e.g. `TestStatusHintAcrossViews`, `TestQuestEmblemsStableAcrossCaret`).
