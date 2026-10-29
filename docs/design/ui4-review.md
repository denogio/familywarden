# UI4 — administrator installed-app browser

## Contract and scope

`admin_installed` is an administrator-only read behind the existing host session/ACL,
not a phone-signed operation or a new approval authority. Query fields are `profile`,
`search`, `filter` (all/limit/unlimited/blocked), and zero-based `page`. Search is a
case-insensitive label/package substring over the complete aggregate before paging.
Numbered pages normally contain 20 apps and are also capped at 48 KiB per reply.
`counts` describes the complete saved-rule inventory, independent of search/filter;
`total` describes the matching set, and `start` is its zero-based row offset.
`pages`, `page`, `snapshot`, `apps_rev` and per-phone `sources` accompany each page.
The snapshot identifies inventory/policy observations, not usage freshness or a write token.
Existing configuration CAS remains the only rule-save authority.

The shared aggregation preserves current-key/profile inventory scoping and configured
but unreported packages. Pending rows reuse existing core new-app observations/actions;
they are not evidence that a package is installed. Usage joins current registered keys
for today without expanding the separate top-used usage summary. Missing usage is not
rendered as zero. A missing/partial phone snapshot remains explicit.

Done changes the local rules draft; Cancel/Escape discard modal edits. Existing Save
rules, confirmation, CAS and pact guards remain in force. Custom durations retain
non-chip values; an existing zero limit is preserved. Duplicate saved entries are left
untouched unless explicitly changed, with an explanatory warning. Per-day shared-budget
fields are not flattened. Counts remain saved-rule counts while pills mark unsaved edits.
If inventory is unavailable, configured rules remain editable with an explicit warning.

## Local validation

- `scripts/test-syntax.sh`: passed, including advertisement, dispatch and both read ACLs.
- `cd core && go test ./...`: passed.
- `node --test test/web-ui.test.cjs`: 8 passed (including EN/DA resource parity).
- Local neutral-host Playwright control tests: 34 passed, 1 existing opt-in skip.
  Additional UI4 scope-switch/pact and immediate approval tests: 2 passed.
  Final EN/DA selection/caret/Cancel/Done checks: 2 passed.
- `FW_TEST_SHARED_UI=1 go test ./internal/service -run '^TestPortableSharedUI$' -count=1`:
  passed against the real portable service and recording adapter, with no router.
- `test/e2e/icon-coverage.cjs`: passed with `FW_CONTROL_PROTOTYPE` supplied externally.
  Exercises 210 icons, bounded transport pages, missing/invalid fallbacks, dark-edge
  pixel sampling at DPR 2 and panel/modal screenshots.

Visual captures were reviewed locally under `/tmp/fw-ui4-visual`: `control-apps-dark-dpr2.png`,
`control-rule-modal-dark-dpr2.png`, `prototype-apps-dpr2.png`,
`prototype-rule-modal-dpr2.png`, and `dark-icon-closeup-dpr2.png`.
Final captures use the latest externally supplied prototype revision (3), replacing revision (2).
The panel follows its search/count-header and numbered-pagination reference;
the modal follows the prototype's scope, radio-card and duration-chip hierarchy, with
additional explicit draft-save explanation, package identity and custom duration.
Search now includes the prototype magnifier and profile-specific placeholder, the pager
uses its inverse active-page treatment, and matching totals are announced politely.
The explicit approved contract still wins over mock data: 20 apps per page, server-side
label/package search and full-inventory saved-rule counts independent of the query.
After these presentation changes, syntax/resource checks, all five UI4 browser tests
and the visual/icon driver passed again.
The prototype path is never a project default; supply it only through
`FW_CONTROL_PROTOTYPE`. Screenshots are local artifacts, not committed fixtures.

The pale-rim regression injects hostile host image border/padding/max-width defaults.
Transparent image/tile backgrounds, explicit zero border/padding, rounded clipping
and cover sizing keep opaque normalized artwork at the tile edge. Pixel sampling
verifies the fictional green artwork reaches the top-center edge on a dark background;
missing/invalid images still keep their existing initials fallback.

No live router, QEMU pact, deployment, tags, releases, signing keys, commits or pushes
were used. Shell integration assertions were added but the container integration suite
and native LuCI browser suite were not run by the worker. Independent review completed;
all three verified findings were fixed. The main session inspected panel/modal DPR2
captures and ran final affected Go race tests and vet, reusing unchanged passing checks.
Native packaged deployment checks are a separate supervisor gate.

## Clean navigation follow-up

Rules navigation now uses the same semantic configuration comparison as the unsaved
changes bar, rather than treating the existence of a draft or an input event as an
edit. Comparison normalizes optional empty collections and ordering without changing
the submitted configuration; app-rule identities survive switching away and restoring
a saved rule. Opening, refreshing, searching, paging, changing browser filters and
cancelling a modal remain clean. Reverting actual edits restores a clean draft.
Settings drafts also have a loaded comparison baseline. Existing non-configuration
form guards remain unchanged.

Real unsaved changes still block navigation when the administrator declines the
explicit localized “Discard unsaved changes and leave this page?” confirmation.
Six focused regression cases passed, covering untouched navigation/polling in EN/DA,
query/page/filter-only navigation, modal Cancel, field/app-rule edit-and-revert and
accept/decline of the real-edit guard in both languages. The consolidated neutral-host
suite then passed with 42 tests and one existing opt-in skip. No private screenshot
content was copied into fixtures or documentation.

## Fresh-review fixes

All three verified P2 findings were addressed; none were rejected:

- The modal now follows the same blocked-before-always-allowed precedence as the
  list and compiler. Its regression also exposed hidden custom minutes being
  re-enabled by the shared control updater, preventing a blocked zero-minute rule's
  unchanged Done action. Inactive duration input locking now survives that updater.
- Newly added local app rules outside the current server results remain visible in
  a separate, explicitly unsaved group with edit and remove controls. This group
  remains available across query/page changes and never changes saved-rule counts
  or server pagination. It also covers local always-allowed additions.
- The core's `app_control_off` refusal now receives a localized explanation about
  enabling and saving app control, with or without configured-rule fallback rows.

Six new browser cases cover precedence/no-op Done/explicit rule changes, a manual
rule added on a later page then edited/searched/removed without saving, and the
app-control-off explanation in both locales with and without configured rules.
The consolidated neutral-host suite passed with 48 tests and one existing opt-in
skip. After a wording clarification, the manual-rule case passed again. Final
syntax/resource/ACL checks and whitespace/no-staged-files checks passed. Go and
icon rendering code were unchanged in this review-fix pass; their earlier passing
validation remains applicable. No router or QEMU environment was accessed.
