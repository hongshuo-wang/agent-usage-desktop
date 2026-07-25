# Query Context Filter Design

## Goal

Replace the current mixed filter rows on the dashboard and sessions page with one consistent query interaction. The default view should stay compact while making the active query unmistakable. Dashboard drill-downs and session browsing must use the same query semantics.

## User-facing design

The top of both pages uses a shared `Current query` bar.

- Primary summary: time range and Agent/source, for example `Last 7 days · All agents · Jul 19–25`.
- Secondary active chips: only non-default conditions are shown as removable chips. Project and model are optional and appear here when active.
- `Edit query` opens a right-side popover/drawer without moving the dashboard content. The drawer groups controls as `Time range`, `Agent`, `Project`, and `Model`.
- The drawer has explicit `Cancel` and `Apply query` actions. Applying updates the URL and triggers the relevant data requests; cancelling leaves the current query unchanged.
- Refresh remains an icon/text action outside the query editor.
- Trend granularity is chart-local and stays beside the Token trend heading. It is not part of the cross-page query.

The same query bar is used on Sessions. When navigating from a dashboard project/model/source row, the sessions page receives those values in the URL and displays them as active chips. Removing a chip updates the URL and reloads the session list. A clear-all action restores the default time range and removes contextual filters.

## Query state and data flow

`UsageFilters` remains the canonical client state: `preset`, `from`, `to`, `source`, `model`, and `project`. The URL is the source of truth for navigation and drill-down; local storage only provides initial preferences for preset, source, and custom dates.

The dashboard and sessions page must use the same serialization and parsing helpers. Dashboard API requests should include the active `model` and `project` whenever the endpoint supports those dimensions. If an aggregate endpoint cannot apply a filter, the UI must not imply that it did; the query bar should remain the shared navigation state and the page should expose a clear limitation message.

`granularity` is persisted separately as a dashboard/chart preference and must not be copied into session URLs.

## Project naming

The backend's existing effective-project precedence remains: explicit session project, usage-record project, cwd basename, then session ID. The UI should distinguish readable names from the final fallback:

- Use the readable project value as the primary label when it comes from project metadata or cwd.
- Group records whose only stable value is a session ID under the localized label `Unnamed project` / `未命名项目`.
- Show the raw session ID as secondary detail or in the session inspector, never as the primary project-ranking label.
- Keep the underlying project key in navigation so a drill-down remains exact.

## Component boundaries

- `TimeRangeSelector` becomes the shared query-summary/editor component. It owns presentation and draft values, while page components own committed `UsageFilters` and fetches.
- A small query-chip/presentation helper formats active conditions and accessible labels for both pages.
- Dashboard keeps chart-local controls near the chart and passes committed filter changes to existing fetch logic.
- Sessions removes the separate model/project input row and renders the same query editor plus inherited/drill-down chips.
- Locale files add labels for query summary, editor sections, apply/cancel, active filters, unnamed projects, and filter limitations.

## Interaction and accessibility

- All controls are keyboard reachable; opening the editor moves focus to its heading or first control and Escape cancels.
- Every chip has a named remove button.
- The active query summary uses a labelled region and announces changes through existing loading/error states.
- Long project/model values truncate visually but remain available via title/accessible name.
- The layout is responsive: the editor becomes a full-width bottom sheet or modal on narrow screens; the summary wraps without horizontal scrolling.

## Error and loading behavior

Applying an invalid custom range keeps the editor open and shows the existing date validation error. On fetch failure, retain the committed query and show the existing retry affordance. Updating one query field aborts obsolete requests as current pages already do.

## Verification

- Component tests cover summary rendering, opening/cancelling/applying the editor, chip removal, keyboard Escape, and responsive-safe labels.
- Dashboard tests verify URL serialization and that project/model drill-down context is preserved into `/sessions`.
- Sessions tests verify the same query controls send identical parameters and that clearing a chip reloads the list.
- Existing backend and locale tests must continue to pass; add focused endpoint coverage if model/project filtering is extended.

## Out of scope

Saved queries, multi-select projects, arbitrary boolean filters, and changing the backend's effective-project precedence are not part of this redesign.
