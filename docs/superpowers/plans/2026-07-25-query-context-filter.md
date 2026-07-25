# Query Context Filter Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the mixed dashboard/session filter rows with a compact shared query bar, an explicit editor, consistent URL-driven filters, and readable project labels.

**Architecture:** Keep `UsageFilters` as the committed query state and URL serialization as the cross-page contract. Refactor `TimeRangeSelector` into a shared summary/editor component with draft state; keep chart granularity local to Dashboard. Extend presentation helpers so project rows can distinguish readable names from session-ID fallbacks.

**Tech Stack:** React 18, TypeScript, React Router, Tailwind CSS v4, Vitest, Testing Library, existing Go REST API.

---

### Task 1: Add query presentation helpers and locale strings

**Files:**
- Create: `src/lib/queryPresentation.ts`
- Test: `src/lib/queryPresentation.test.ts`
- Modify: `src/lib/locales/en.json`
- Modify: `src/lib/locales/zh.json`

- [ ] **Step 1: Write failing helper tests** for summary labels, active chips, and ID-only project fallback.
- [ ] **Step 2: Run `npm test -- --run src/lib/queryPresentation.test.ts`** and confirm the new module is missing.
- [ ] **Step 3: Implement pure helpers** with stable output keys: `formatQuerySummary`, `getActiveQueryChips`, and `presentProjectKey`.
- [ ] **Step 4: Add localized labels** for query summary/editor sections, apply/cancel, active filters, unnamed projects, and unsupported aggregate filters.
- [ ] **Step 5: Run the focused test and locale test** and commit as `feat: add query presentation helpers`.

### Task 2: Refactor `TimeRangeSelector` into the shared query bar/editor

**Files:**
- Modify: `src/components/TimeRangeSelector.tsx`
- Modify: `src/components/TimeRangeSelector.test.tsx`
- Modify: `src/styles/globals.css` only if a drawer animation/focus style cannot be expressed with existing utilities.

- [ ] **Step 1: Add failing component tests** for collapsed summary, opening editor, cancel/escape, apply, removable chips, and compact chart-local granularity rendering.
- [ ] **Step 2: Run the focused component tests** and confirm failures against the current two-row selector.
- [ ] **Step 3: Implement committed-vs-draft props**: receive `UsageFilters` values, expose `onApply`/`onClear`, and keep granularity as an optional local control rendered outside the editor.
- [ ] **Step 4: Implement the right-side editor** with grouped controls, focus entry, Escape cancellation, responsive full-width mobile presentation, and accessible chip remove buttons.
- [ ] **Step 5: Run focused component tests and `npm test -- --run src/components/TimeRangeSelector.test.tsx`**; commit as `feat: add shared query editor`.

### Task 3: Wire Dashboard to the shared query contract

**Files:**
- Modify: `src/pages/Dashboard.tsx`
- Modify: `src/pages/Dashboard.test.tsx`
- Modify: `src/lib/usageFilters.ts` only if serialization needs a shared helper adjustment.

- [ ] **Step 1: Add failing Dashboard tests** proving the summary opens the editor, applied project/model values are serialized to the sessions drill-down URL, and chart granularity is not included in that URL.
- [ ] **Step 2: Update Dashboard state handlers** so editor apply commits filters, persists initial preferences, and triggers the existing fetch effects without duplicating state.
- [ ] **Step 3: Replace the old context aside** with active query chips and a clear-all action. Keep collection-status messaging separate.
- [ ] **Step 4: Move trend granularity next to the Token trend heading** and remove it from the global filter row.
- [ ] **Step 5: Pass model/project values into supported aggregate requests; if the current endpoint contract cannot filter them, render the localized limitation note instead of implying filtered metrics.**
- [ ] **Step 6: Run Dashboard tests and commit as `feat: unify dashboard query state`.**

### Task 4: Wire Sessions to the same shared query editor

**Files:**
- Modify: `src/pages/Sessions.tsx`
- Modify: `src/pages/Sessions.test.tsx`

- [ ] **Step 1: Add failing tests** showing identical query controls, inherited project/model chips, chip removal, and clear-all URL behavior.
- [ ] **Step 2: Remove the standalone model/project input row** and render the shared query bar/editor plus inherited context in one location.
- [ ] **Step 3: Apply editor changes through the same URL/filter helper** used by Dashboard; keep `sessionParams` as the only request parameter builder.
- [ ] **Step 4: Preserve mobile session navigation and selected-session behavior** while the editor opens as a full-width sheet on narrow screens.
- [ ] **Step 5: Run Sessions tests and commit as `feat: share query filters with sessions`.**

### Task 5: Make project ranking labels readable without breaking exact drill-downs

**Files:**
- Modify: `src/pages/Dashboard.tsx`
- Modify: `src/components/sessions/SessionList.tsx`
- Modify: `src/lib/sessionPresentation.ts`
- Test: `src/lib/sessionPresentation.test.ts`
- Modify: `src/pages/Dashboard.test.tsx`

- [ ] **Step 1: Add failing tests** for readable cwd/project names, ID-only values rendering as localized unnamed projects, and raw IDs remaining in navigation/detail data.
- [ ] **Step 2: Implement a presentation-only project label** that preserves the backend key for `onSelect` while changing only the visible primary/secondary text.
- [ ] **Step 3: Update dashboard breakdown rows and session list labels** to use the helper with `title` attributes for truncated details.
- [ ] **Step 4: Run focused presentation, Dashboard, and Sessions tests and commit as `fix: clarify project labels`.

### Task 6: Full verification and cleanup

**Files:**
- Modify only files needed for formatting or test fixes.

- [ ] **Step 1: Run `npm test -- --run` and `npm run build`**; resolve TypeScript, accessibility, and locale failures.
- [ ] **Step 2: Run `go test ./...`** to verify any API/query contract changes.
- [ ] **Step 3: Run `git diff --check` and inspect the final diff** to ensure existing unrelated worktree changes remain untouched.
- [ ] **Step 4: Run the local dev server and verify desktop and narrow layouts manually**: collapsed query, editor open, apply/cancel, project drill-down, and session chip removal.
- [ ] **Step 5: Commit only implementation changes with conventional commit messages and report test evidence.**
