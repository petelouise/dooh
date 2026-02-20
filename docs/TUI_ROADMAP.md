# dooh TUI Roadmap (Prioritized)

## Context
Current pain points from active use:
- footer key hints are not consistently visible/useful,
- top title bar background does not reliably fill full terminal width,
- column headers/rows can still look visually off in some terminals,
- theme combinations are not consistently attractive/readable,
- filtering/sorting ergonomics are not explicit enough for power use,
- task metadata is missing `description` and `url(s)`,
- `groups` should be removed from UX (and eventually data model usage).

This roadmap is ordered to maximize usability gains while minimizing regressions.

## Priority order

## P0: Stability + Readability Baseline (do first)
Goal: make the existing TUI trustworthy before adding scope.

### Scope
- Restore persistent bottom help/hotkey line in a guaranteed reserved footer region.
- Ensure title/filter/footer background blocks always fill exact terminal width.
- Fix table alignment for all rows/headers under ANSI styling and Unicode symbols.
- Remove any remaining render-path flicker triggers.

### Implementation
- Move all line composition to plain-text segments first, apply style last.
- Add ANSI-aware width helpers (`visibleWidth`, `truncateVisible`, `padVisible`) and stop using raw width on styled strings.
- Reserve fixed layout regions:
  - header: title + banner + filters + tabs + counts + column header
  - footer: selected summary + hotkeys
  - body: viewport-managed rows
- In Bubble Tea view, render only after `WindowSizeMsg`; reflow on resize only.

### Acceptance
- Header columns align with row columns at widths 80/100/140.
- Footer hotkeys visible at all times (unless terminal too short, then selected-summary is truncated first).
- Top bar background spans full width.
- No idle flicker.

## P1: Filtering + Sorting UX (high impact, moderate effort)
Goal: make complex filtering fast and obvious.

### Scope
- Keep existing AND-combined filter semantics across groups.
- Add tokenized quick filter syntax in text field:
  - `#tag`
  - `~area`
  - `^goal`
  - `@assignee`
  - `!overdue`
- Support quoted multi-word tokens:
  - `#\"Deep Work\"`
  - `~\"Personal Ops\"`
- Add sort controls:
  - sort by `priority` (now -> soon -> later),
  - sort by `scheduled` (earliest first, then unscheduled).

### Implementation
- Introduce parsed filter AST in TUI model:
  - free-text fuzzy term(s),
  - typed token filters,
  - implicit AND combination.
- Add top-bar chips reflecting parsed tokens.
- Add sort chip + keybinding (`o` cycles sort mode).
- Keep facet dropdown for tags/assignee; add area/goal facets in same framework later.

### Acceptance
- Combined filters remain deterministic (AND).
- Token filtering works with mixed text + chips.
- Sort changes order without breaking selection/expand behavior.

## P2: Task schema + detail richness
Goal: support richer task context for human and AI.

### Scope
- Add `description` field to tasks.
- Add `urls` field to tasks (store as newline-delimited text for MVP; parse/render as list).
- Show description + URLs in expanded task view.
- Remove `groups` from TUI output and docs.

### Implementation
- Migration:
  - `tasks.description TEXT NOT NULL DEFAULT ''`
  - `tasks.urls TEXT NOT NULL DEFAULT ''`
- CLI:
  - `task add --description --url` (repeatable `--url`)
  - `task update --description --url --clear-urls`
- TUI:
  - include `description` and `urls` in detail card.
  - remove `groups` row; keep `areas`, `projects`, `goals`, `tags`.

### Acceptance
- New fields persist and are queryable.
- Expanded task card shows clean URL list.
- No `groups` references in TUI/help/docs.

## P3: Area navigation and IA improvements
Goal: faster collection navigation from top-level.

### Scope
- Add numeric area shortcuts in top bar (`6-9` cycling first four visible areas, plus paged list).
- Add dedicated `areas` view with completion and counts (like project/goal views).
- Enter on area row scopes tasks by that area.

### Implementation
- Add `areas` to tab strip and view state.
- Reuse progress-row loader by `kind='area'`.
- Update selected summary and scope chip to show `scope:area:<name>`.

### Acceptance
- Area navigation is discoverable and consistent with project/goal behavior.
- Scope drill-in/out works identically.

## P4: Theme system redesign (beauty pass)
Goal: make themes consistently attractive and legible.

### Scope
- Replace ad-hoc palette values with semantic theme tokens only.
- Add theme validation tests:
  - min contrast ratio for key surfaces,
  - warn on clashing accent pairs.
- Add at least 6 curated themes (2 light, 4 dark) tuned for low harshness.

### Implementation
- Add `theme lint` utility (internal test helper).
- Remove mixed fallback to hardcoded 256-color constants where possible.
- Use consistent semantic mapping:
  - text/muted/accent/success/warn/danger/chart1-4.

### Acceptance
- All shipped themes pass contrast tests.
- Readability remains strong with full background styling.

## P5: Bigger Bubble Tea upgrades (optional, only after P0-P2)
Goal: leverage Bubble Tea more without destabilizing core.

### Scope
- Migrate table/body rendering to `bubbles/viewport` + controlled virtual list.
- Add command palette (`:`) for discoverable actions.
- Add lightweight animations on interaction only (never idle):
  - expand/collapse transitions,
  - chip select flash.

### Risk controls
- Keep `--renderer legacy` as fallback.
- Add snapshot tests for layout at multiple widths.
- Feature-flag new interactions behind `--ui-experimental` initially.

## Suggested execution sequence
1. P0
2. P1
3. P2
4. P3
5. P4
6. P5 (optional)

## Notes on “are we leveraging Bubble Tea enough?”
Short answer: not yet.
- Current Bubble Tea use is mostly event loop + key handling.
- Bigger gains come from adopting Bubble Tea/Bubbles primitives for layout, viewport, and interactive controls.
- Do that only after P0 baseline is fully stable.
