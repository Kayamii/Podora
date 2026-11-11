# Design

Podora's interface is a clean, neutral operations console: a fixed sidebar,
white cards on a light grey canvas (or the dark equivalent), one blue accent,
and status colours used only for resource state. Tokens live in
`frontend/src/index.css`; shared components in `frontend/src/components/ui.tsx`.

## Principles

1. **Calm by default.** Neutral greys carry the interface. Nothing is
   coloured unless it means something.
2. **One accent.** Blue marks actions, links, the current page and focus.
3. **Status is reserved.** Green, amber and red only describe resource state,
   and always appear next to a text label (`Status`, `Badge`), never alone.
4. **Verdict first.** The overview opens with a health summary that lists each
   issue as a link to where it can be investigated.
5. **Every list leads somewhere.** Rows open the resource's own page; pages
   link back through a breadcrumb and across to related resources.

## Tokens

| Token | Light | Dark | Role |
|---|---|---|---|
| `--canvas` | `#f6f7f9` | `#0c0d10` | Page background |
| `--surface` | `#ffffff` | `#14161a` | Cards, sidebar, dialogs |
| `--subtle` | `#f1f3f5` | `#1b1e23` | Hover rows, chips, segmented track |
| `--line` / `--line-strong` | `#e3e6ea` / `#cdd2d9` | `#25282f` / `#363a43` | Dividers / control borders |
| `--fg` / `--fg-2` / `--muted` | `#14171c` / `#4a515c` / `#69717d` | `#eceef1` / `#b3b9c2` / `#8a919c` | Text |
| `--accent` | `#2459d9` | `#6d98f5` | Actions, links, current page |
| `--ok` `--warn` `--danger` | `#1a9a55` `#d98a0b` `#d6392f` | `#3fbf7a` `#e5a52e` `#ee6258` | State dots and fills |
| `--*-fg` / `--*-soft` | | | State text and pill backgrounds |
| `--chart-2` | `#7a45d6` | `#a986f0` | Second chart series (memory) |
| `--code-bg` / `--code-fg` | `#12151a` / `#d6dae1` | `#0a0b0e` / same | Log viewer |

Light and dark follow `prefers-color-scheme`.

## Type

- **Inter** (variable) for everything; base size 15px, line height 1.5.
- **JetBrains Mono** (variable) only for logs, image names, IPs, ports and
  labels: things that are data, not prose.
- Tabular numerals in every numeric column.
- Scale: 13px table headers and field labels, 15px body, 18px card verdicts,
  24px page titles, 30px stat values.
- Both fonts are self-hosted through `@fontsource-variable`.

## Components

- **Sidebar** (`AppLayout`): grouped sections (Cluster, Workloads,
  Networking) with Lucide icons at 17px / 1.75 stroke. Current page: accent
  text on `--accent-soft`. Becomes a drawer below 1024px.
- **PageHeader**: breadcrumb, title, status badge, one-line subtitle, actions
  on the right.
- **Card**: 1px border, 8px radius, faint shadow, optional header row with a
  title and a right-aligned link. Tables run edge to edge (`padded={false}`).
- **Tables** (`styles.ts`): 13px muted headers, rows divided by `--line`.
  The first cell's link stretches over the row so the whole row is clickable.
- **Status / Badge**: 8px dot plus label in tables; tinted pill in headers.
- **Button**: primary (accent), secondary (outlined), danger, danger-outline.
  All 36px tall. Destructive actions use danger-outline until confirmed.
- **Segmented, Switch, Field, `.input`**: 36px controls with a 13px label
  above.
- **ConfirmDialog**: native `<dialog>`; every change to the cluster goes
  through one, and failures are shown inside it.
- **UsageChart**: single-series area chart, 2px line, 10% fill, three
  gridlines, hover crosshair with a readout. Gaps in sampling stay gaps.
- **Meter**: 6px rounded bar; accent, then amber from 75%, red from 90%.
- **LogViewer**: dark code pane in both themes, follows new lines until the
  reader scrolls up, then offers "Jump to latest".
- **Tabs**: underlined, accent bar on the selected tab, optional count pill.
  Used for views of one resource (a Helm release).
- **ValuesEditor**: YAML in a monospace textarea with a line-number gutter;
  Tab indents two spaces. Read-only mode for reference panels.
- **CodeBlock**: dark pane like the log viewer, with a Copy button.
- **Sticky action bar**: long forms (install, upgrade) keep Preview and the
  primary action pinned to the bottom of the viewport.
- **Log levels**: in the log pane, errors get a red tint with a red left bar,
  warnings an amber tint with an amber bar, debug lines are dimmed; info is
  plain. Levels come from `services/logLevel.ts`, which reads plain words,
  logfmt, JSON, bracketed and klog formats, and lets stack frames inherit the
  level of the line above.
- **Skeleton / EmptyState**: lists show grey placeholder rows while loading
  and an icon, a sentence and a call to action when empty.
- **Sortable tables**: header buttons with an arrow on the active column and
  `aria-sort`; numeric columns sort heaviest first.

## Rules for new screens

- New resource types get a list page (filters on top: namespace, search, then
  type-specific segmented filters) and, if they can be acted on, a detail page.
- Do not introduce new colours. Emphasis comes from weight and size.
- Icons come from `lucide-react`, at the sizes above. No emoji.
- Kubernetes terms keep their names (Pod, Deployment, Namespace).
