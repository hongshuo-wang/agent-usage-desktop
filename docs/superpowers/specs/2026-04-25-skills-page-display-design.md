# Skills Page Display Design

## Goal

Improve the `Skills` page so users can:

- scroll through the available skills list without content being clipped,
- immediately understand which CLI each skill belongs to,
- open a skill's source directory from a clear, prominent action.

The change should stay local to the existing desktop frontend, preserve the current edit workflow on the right side, and avoid backend/API changes.

## Scope

This design covers the left-side skills list in `src/pages/config/Skills.tsx` and its interaction with the existing editor panel.

In scope:

- fix the left-list scrolling behavior,
- regroup the visible skills list by CLI target,
- render the same skill in multiple groups when multiple CLI targets are enabled,
- improve the card information hierarchy,
- make the source-directory action more obvious,
- keep the current right-side editor behavior intact.

Out of scope:

- backend schema or API changes,
- opening target install directories,
- search, filtering, sorting controls beyond fixed group ordering,
- redesigning the inventory/discovered/conflicts sections,
- changing how skills are created, saved, deleted, or synced.

## Current Problems

The current page has three UX issues:

1. The left-side available-skills list can be visually clipped by the page layout, making lower items unreachable or hard to discover.
2. Each skill card shows target checkboxes, but the primary scan path does not make it easy to answer “which CLI is this for?” at a glance.
3. Opening the source directory is possible today, but the action is visually secondary and easy to miss.

The page already has a workable two-column editing flow, so the redesign should solve these issues without replacing the overall structure.

## Principles

- Preserve familiar layout: keep the current list-on-the-left and editor-on-the-right structure.
- Make CLI grouping the first-level information hierarchy.
- Keep selection semantics tied to the underlying skill record, not to a duplicated visual card.
- Fix layout at the root cause instead of masking the clipping with extra wrappers.
- Prefer front-end-only derivation over backend changes for this iteration.

## Recommended Approach

Use a grouped list in the left column, ordered by CLI:

1. `Codex`
2. `Claude Code`
3. `OpenCode`
4. `OpenClaw`
5. `Unassigned` (only when needed)

Within each group, show skill cards for all skills that have that CLI target enabled. If a skill is enabled for multiple CLI targets, render it once in each relevant group.

If a skill has no enabled CLI targets, render it once in a trailing `Unassigned` section so it remains discoverable and editable.

The right editor stays structurally unchanged. Clicking any grouped card still selects the same underlying skill record and loads it into the existing form.

This approach directly matches the approved UX direction:

- users identify CLI ownership before reading card details,
- multi-CLI skills remain discoverable in each relevant section,
- the page becomes easier to scan without changing backend data structures.

## UI Structure

### Top-Level Layout

Keep the current page shape:

- inventory/status sections at the top,
- two-column workspace below,
- grouped skills list on the left,
- existing editor on the right.

The two-column section remains the main working area.

### Left Column

The left column becomes a grouped, independently scrollable list with:

- existing header actions such as `Create`,
- one section per CLI that currently has at least one enabled skill,
- an `Unassigned` section when one or more skills have no enabled targets,
- a count badge per group,
- one skill card per matching skill.

Empty groups are hidden to reduce noise.

### Right Column

The right editor remains the current form:

- same selected skill state,
- same fields,
- same target toggles and sync method controls,
- same save/delete flow.

No interaction contract changes are required on the editor side.

## Card Design

Each grouped card should present information in this order:

1. skill name,
2. short description,
3. status badge (`Enabled` or `Disabled`),
4. CLI and sync-method chips,
5. source path,
6. prominent `Open Source` action.

This makes the card easier to scan because the most useful distinctions appear higher in the visual hierarchy.

### `Open Source` Action

The existing `openFolder(skill.source_path)` behavior stays the same, but the action should be visually upgraded from a low-emphasis inline control to a primary card action.

Interaction rule:

- clicking the card selects the skill,
- clicking `Open Source` opens the source directory only,
- the button must stop propagation so it does not accidentally change selection.

## Data and State Model

No backend changes are needed. The page continues to consume the existing `Skill[]` response from `GET /api/config/skills`.

Add a front-end-only derived structure for rendering grouped cards. Conceptually:

- source of truth: raw `skills` array,
- derived view model: grouped entries by CLI target,
- selected state: still driven by `selectedID` and the raw skill record.

The grouped view model should be treated as display-only data. It must not replace the original record used by the editor.

### Group Derivation Rules

For each tool in `TOOLS`:

- include a group only if at least one skill has that target enabled,
- include a visual card for every skill where `skill.targets[tool]?.enabled` is true,
- reuse the same underlying `skill.id` for selection.

Additionally:

- if a skill has no enabled targets at all, include it once in `Unassigned`,
- place `Unassigned` after all CLI groups.

This means one skill can appear in multiple CLI groups while still mapping to one editor state, or exactly once in `Unassigned` when it has no enabled targets.

## Layout and Scrolling

The scrolling bug should be fixed through layout constraints, not through page-level overflow hacks.

The left list area must be the actual scroll container. The parent chain for the two-column section and left pane should preserve `min-h-0` so the browser allows the inner list to shrink and scroll correctly.

Target behavior:

- the left grouped list scrolls independently,
- the right editor also scrolls independently,
- the page does not clip the bottom of the skills list,
- the inventory summary above remains stable.

## Implementation Notes

Frontend implementation is expected to stay concentrated in `src/pages/config/Skills.tsx`, with optional minor translation updates in:

- `src/lib/locales/en.json`
- `src/lib/locales/zh.json`

Recommended implementation slices:

1. add helper logic to derive grouped display data from `skills`,
2. replace the flat left list render with grouped sections,
3. restyle cards to emphasize CLI and source access,
4. verify container sizing and independent scroll behavior.

If `Skills.tsx` becomes noticeably harder to read, small extraction helpers are acceptable, but a large refactor is unnecessary for this scope.

## Error Handling and Edge Cases

- If no skills exist, keep the existing empty state.
- If a skill has no enabled targets, it should appear in `Unassigned` so it remains selectable and editable.
- If a selected skill appears in multiple groups, all duplicated cards representing that `skill.id` should render as selected.
- If opening the source directory fails, reuse the existing error banner behavior.
- Long paths and long descriptions should continue to wrap or clamp without breaking card height badly.

## Testing

### Frontend Behavior

Validate these cases:

- enough skills exist to require scrolling and the left list remains reachable,
- a skill enabled for multiple CLI targets appears in multiple groups,
- clicking any grouped instance selects the correct skill in the right editor,
- clicking `Open Source` opens only the folder action and does not trigger card selection,
- the right editor still works for save/delete/edit operations,
- the grouped layout remains readable in both English and Chinese.

### Visual Checks

Confirm:

- group headers remain readable in light and dark themes,
- badges and chips do not dominate the card visually,
- cards remain usable when descriptions are missing,
- grouped sections do not cause the two-column layout to collapse unexpectedly.

## Approved Decisions

- Use CLI-grouped presentation in the left list.
- Render the same skill once per enabled CLI group.
- Keep the current two-column page structure.
- Keep the current right-side editor structure and behavior.
- Promote source-directory access to a clear `Open Source` card action.
- Limit the change to a frontend display/layout improvement with no backend changes.
