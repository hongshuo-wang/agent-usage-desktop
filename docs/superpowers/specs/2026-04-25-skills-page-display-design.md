# Skills Page Display Design

## Goal

Redesign the `Skills` page in `src/pages/config/Skills.tsx` so the page:

- makes the current CLI context obvious at a glance,
- gives the skills list and editor natural independent scrolling,
- improves the overall visual hierarchy without changing backend APIs,
- preserves the existing save, delete, import, and conflict-resolution flows.

The approved direction is **CLI First**: users choose a CLI first, then view and edit skills in that CLI context.

## Scope

This design covers the `Skills` frontend page only.

In scope:

- reshape the page into a CLI-first workspace,
- reduce the visual weight of the inventory summary area,
- add an explicit CLI switcher for `Claude Code`, `Codex`, `OpenCode`, and `OpenClaw`,
- regroup the left list into `Connected`, `Not Connected`, and `Unassigned` for the current CLI,
- improve card information hierarchy so current-CLI status is primary,
- keep the right editor focused on current-CLI configuration first,
- fix scrolling and responsive behavior for narrow windows,
- add lightweight frontend-only search within the current CLI view.

Out of scope:

- backend schema, API, or migration changes,
- new sync semantics or new tool adapters,
- batch editing, drag-and-drop ordering, or multi-select actions,
- new import sources,
- introducing a frontend test framework.

## Current Problems

The current page has three user-visible issues:

1. The page does not clearly answer “which CLI is this skill for?” because CLI information is mixed into the card instead of defining the page structure.
2. The content feels heavy and visually noisy because inventory details, list items, badges, paths, and editor content compete at the same level.
3. Scrolling does not feel natural in constrained windows because the page asks users to manage too much vertically before reaching the actual workspace.

The existing data model and CRUD flow are already usable, so the redesign should solve the display and navigation problems without reworking backend behavior.

## Approved UX Direction

The user approved the following decisions during brainstorming:

- Use a **CLI First** layout rather than a skill-first layout.
- Prioritize:
  1. clear CLI ownership,
  2. reliable scrolling in small windows,
  3. cleaner visual presentation.
- In each CLI view, group skills as:
  - `Connected`,
  - `Not Connected`,
  - `Unassigned`.
- Keep the redesign frontend-only and preserve current save/delete/import/conflict flows.

## Principles

- Make the selected CLI the primary context for the whole page.
- Keep the working area compact and scannable before showing deeper details.
- Treat current-CLI state as primary information and other-CLI state as secondary context.
- Fix overflow at the layout level, not by adding page-level scroll hacks.
- Reuse existing data and behaviors wherever possible.

## Information Architecture

### 1. Lightweight Summary Strip

The current top inventory section should be compressed into a lighter summary strip instead of a large dominant card.

It should include:

- global library path and CLI availability status,
- four summary counters:
  - `Library`,
  - `Discovered`,
  - `Importable`,
  - `Conflicts`,
- primary actions:
  - `Refresh`,
  - `Import non-conflicting`.

Detailed discovered items and conflict entries should move behind lower-emphasis expandable sections so they are still available but do not push the main workspace downward by default.

### 2. CLI Switcher

Below the summary strip, add a prominent CLI switcher with four tabs:

- `Claude Code`
- `Codex`
- `OpenCode`
- `OpenClaw`

Each tab should show:

- tool label,
- count badge for skills currently connected to that CLI,
- active visual state.

Selecting a tab changes the left list grouping and the right-side context. The page should choose a stable default CLI on initial load:

- first CLI with at least one connected skill,
- otherwise fall back to `Codex`.

### 3. Main Workspace

Keep a two-pane workspace on wide screens:

- **left:** current-CLI skill list,
- **right:** selected skill details and editor.

The workspace remains the main focus of the page. The summary strip and CLI switcher should support the workspace, not dominate it.

## Current-CLI List Design

### Grouping Rules

Within the selected CLI, derive three groups from the existing `skills` array:

1. **Connected**
   - skill has `targets[currentCLI]?.enabled === true`
2. **Not Connected**
   - skill has at least one enabled target somewhere, but not for the current CLI
3. **Unassigned**
   - skill has no enabled targets at all

This grouping directly answers:

- what is already installed for this CLI,
- what could be attached to this CLI,
- what is not assigned anywhere yet.

### List Controls

At the top of the list pane, include:

- `Create` button,
- lightweight search input for client-side filtering by name, description, or source path.

No advanced sorting or backend-backed filters are needed in this iteration.

### Skill Card Hierarchy

Each list item should emphasize the current CLI state in this order:

1. skill name,
2. current-CLI connection status,
3. short description,
4. current-CLI sync method when connected,
5. global enabled/disabled state,
6. source path as secondary metadata,
7. `Open Source` action.

The card should no longer lead with a mixed bag of all CLI chips. Other CLI memberships may appear as low-emphasis secondary chips if needed, but they must not compete with the current-CLI state.

### Selection Behavior

- Clicking a card selects that skill and loads it into the editor.
- Clicking `Open Source` opens `skill.source_path` only.
- `Open Source` must stop propagation so the click does not also change selection unexpectedly.

## Right Editor Design

The editor keeps the existing CRUD behavior and fields, but its visual ordering changes so the current CLI is the first thing users manage.

Recommended section order:

1. **Basic Info**
   - name
   - description
   - source path
2. **Current CLI Configuration**
   - whether this skill is connected to the selected CLI
   - sync method for the selected CLI
3. **Other CLI Summary**
   - small status chips for the three non-selected CLIs
4. **Global Status**
   - enabled / disabled
5. **Actions**
   - save
   - delete
   - open folder

This keeps the page aligned with the user’s mental model: “I am editing this skill for `Codex` right now,” not “I am editing a generic record and one of these toggles happens to be Codex.”

## Data and State Model

No backend changes are needed.

The page continues using:

- `GET /api/config/skills`
- `GET /api/config/skills/inventory`
- `GET /api/config/files`
- existing mutation endpoints for save, delete, import, and conflict resolution

Add frontend-only derived state:

- `currentCLI`
- filtered/grouped lists for the current CLI
- summary counts per CLI
- optional search query

The raw `skills` array remains the source of truth for selection and mutation. Grouped/current-CLI structures are display-only derivations.

## Layout and Scrolling

Scrolling should be fixed by establishing clear scroll containers inside the existing flex layout.

Target behavior:

- the page itself stays within the viewport work area,
- the summary strip stays compact and above the workspace,
- the CLI switcher stays visible above the workspace,
- the left list scrolls independently,
- the right editor scrolls independently,
- no section depends on page-level overflow to remain usable.

This likely requires preserving or tightening `min-h-0`, `min-w-0`, and explicit overflow boundaries through the `Config` page, `Skills` page, list pane, and editor pane.

## Responsive Behavior

### Wide screens

- two-column workspace,
- left pane for the current-CLI list,
- right pane for the editor,
- both panes scroll independently.

### Medium screens

- still two columns,
- tighter card spacing,
- badges and metadata become more compact.

### Narrow screens

- switch to vertical stacking,
- current-CLI list first,
- editor second,
- use one natural vertical workspace scroll flow instead of forcing two cramped panes.

The redesign should not assume a large desktop monitor; it must remain usable in smaller Tauri windows.

## Expandable Details for Inventory

Discovered skills and conflicts should remain available, but they should no longer be expanded as large default sections at the top of the page.

Recommended treatment:

- render each as a disclosure/expandable section,
- show counts in the header,
- only expand when the user asks for details.

This keeps important inventory information accessible while protecting the main editing workspace from vertical sprawl.

## Implementation Notes

The implementation should stay mostly in `src/pages/config/Skills.tsx`, but small presentational extractions are acceptable if they make the file easier to follow.

Likely implementation slices:

1. add frontend helpers for:
   - per-CLI counts,
   - current-CLI grouping,
   - search filtering,
2. reshape the header/inventory area into a lighter summary strip,
3. add CLI switcher state and rendering,
4. replace the left pane with current-CLI grouped sections,
5. reorder the right editor so current-CLI settings come first,
6. verify scroll containers and responsive breakpoints,
7. add or update translation strings only where the new labels require it.

## Error Handling and Edge Cases

- If there are no skills, keep the existing empty state, but still allow creating a new skill.
- If the selected CLI has no connected skills, show empty `Connected` state without hiding the rest of the page.
- If a skill is in `Not Connected`, the editor should still allow enabling it for the current CLI.
- If a skill is `Unassigned`, it must remain discoverable and editable.
- If opening the source directory fails, reuse the existing error banner flow.
- Long source paths should truncate or wrap gracefully without breaking the list layout.
- English and Chinese labels should remain readable at the chosen card density.

## Validation

Because the repo currently has no dedicated frontend test framework, validation should be pragmatic:

### Build validation

- run `npm run build`

### Manual UI validation

- switch among all four CLI tabs,
- confirm the list re-groups into `Connected`, `Not Connected`, and `Unassigned`,
- resize the window narrower and verify scrolling remains usable,
- confirm the left and right panes scroll independently,
- open a skill source directory,
- edit and save a skill,
- delete a skill,
- preview import and conflict actions to ensure those flows still surface correctly.

## Approved Decisions

- Use a CLI-first page architecture.
- Make CLI switching the primary navigation inside `Skills`.
- Keep summary data visible but visually lightweight.
- Group the current-CLI list as `Connected`, `Not Connected`, and `Unassigned`.
- Keep the current backend/API contract unchanged.
- Preserve existing CRUD, import, and conflict behaviors.
- Fix scrolling through layout cleanup rather than page-level overflow workarounds.
