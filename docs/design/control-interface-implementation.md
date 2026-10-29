# Shared control interface — implementation checkpoints

## Current boundary

The control prototype is a visual reference, not shipped code or product rules.
R5 replaces the old LuCI pages with one core-backed shared workspace:

- Canonical presentation: `web/control.js`, `control.css`, `control-i18n.js` and `family.js`.
- LuCI mount: `luci-app-familywarden/htdocs/luci-static/resources/view/familywarden/control.js`.
- Native RPC/session bridge: `luci-app-familywarden/htdocs/luci-static/resources/familywarden/control-host.js`.
- Backend: existing core identity/requests/usage APIs and authoritative configuration,
  DNS, network-action and pact methods. The host provides authenticated transport.

The host supplies `call(method, params)`, locale, optional enrollment facts and the
native protected-recovery URL. No new password, port, iframe, framework or CDN is
introduced. Product CSS is scoped under `.fwa`; Requests is independently scoped
under `.fwx`, so it cannot restyle its parent workspace. EN/DA tables have key parity.
LuCI owns only its outer frame, login/session, RPC/ACL and mount sizing; another
router host reuses `web/` rather than reimplementing the presentation.

The prototype's mock API shapes are **not** frozen by this work. Existing grant
limits, lifetime, requesting-device/key binding, parent scopes and pact exclusions
remain core decisions. The browser does not grant authority or infer enforcement.

## UI1 presentation corrections (2026-10-29)

Family sections are **Profiles** and **Administrator phones**: the core supplies no
age/kind classification. Reported screen use stays visible without a total budget;
missing usage or budget facts remain unavailable. App rows show uploaded labels with
the stable package ID as secondary text, without app-specific name mappings.

`mount(root, options)` accepts `theme: 'light' | 'dark' | 'auto'` (default `auto`).
The returned `setTheme(value)` changes only the root palette, preserving drafts and
focus. Auto follows OS preference changes; `stop()` removes that listener. The thin
LuCI host translates the explicit HTML `data-darkmode` marker, watches changes, and
cleans up observers/polling when its mount is removed. Hosts without that marker
use auto. Shared modules do not inspect LuCI/theme-specific document attributes.

Deterministic Playwright checks cover EN/DA labels, measured/missing/zero usage,
package identity, explicit/auto themes, host marker translation and cleanup,
Proton-like global element/hover rules and 1440/390/320px layouts. An optional
`FW_PROTON_CSS` local path exercises a public vendor stylesheet on the disposable
neutral host; no vendor CSS, household data or production endpoints are fixtures.
The targeted CSS corrections suppress generic host button shadows/movement, retain
mobile table headers and keep avatar colors scoped. Delayed native attachment does
not stop the workspace before it has been connected; removal then cleans up listeners.
App-identity flex applies only to the label/package wrapper, not its status badge.

Final evidence: 31 neutral/packaged-native browser cases with the public Proton CSS,
8 Node checks, syntax and package smoke pass; fresh review is clean. LuCI r9 is
prepared, not deployed. Read-only browser-local asset replacements against the real
router were inspected in explicit light/dark mode on desktop/mobile; authoritative
configuration is unchanged. Private household data/screenshots remain outside the
repository. The production theme itself was not changed. The audit also found a
management HTTPS hostname mismatch and disabled redirect, tracked separately as OPS1.

## R5 visual and runtime checkpoint (2026-10-28)

The shared workspace uses the prototype's navy rail, light/dark surfaces, attention
inbox, profile cards, delivery pipeline and Ulysses voyage presentation. Desktop and
mobile screenshots were inspected, including the prototype's LuCI-host variant.
Host-width constraints, heading inheritance, optional-node rendering, nested arrays,
section spacing and Requests stylesheet isolation have regression coverage.

Views/editors cover Family/profile rules, Requests/usage/new apps, Devices/enrollment,
Ulysses and Administration. Missing usage is unavailable, not a measured zero;
unknown network status is not an allow claim. Core supplies today's budget from the
same compiled app policy used for phones, including its optional total bonus.

Go race/vet, syntax, seven Node checks, 495 fw4/nft checks, fresh-package smoke and
33 browser cases pass on core r17/backend rc10-r14/LuCI r6. Browser checks include
real import/CAS/confirmation/restart rollback, cumulative pact DNS, no UCI writes
from shared editors, EN/DA, narrow/large-text layouts, draft preservation, fingerprint
approval, frozen weekdays, functional Ulysses/credential actions and late replies.
No production deployment; R6 adapter proof and R7 migration gates remain separate.

## Rules presentation correction (2026-10-28, LuCI r7)

The first R5 visual pass did not port the profile Rules editor: it still exposed a
flat configuration form. Rules now follows the reference's panel hierarchy: draft
notice, daily budget pattern, per-app modes, new-app switch, downtime, home internet
windows and an unsaved-changes save/discard bar. Profile/network details and DNS
remain available below the primary panels instead of preceding them.

All controls edit the existing local configuration draft and submit the same core
CAS/confirmation, DNS-only or amendment operations. App rows use configured/reported
packages; pending approvals remain labelled pending rather than editable allow rules.
No prototype mock data, schedule validation or enforcement logic was copied. Window
inputs retain HH:MM/24:00 support rather than restricting the core's valid endpoints.

The active person tab remains visible, selecting it again does not reset a draft,
and navigation still confirms discard. LuCI clips the rounded frame without creating
an extra scroll container, so the save bar stays visible during editing. Actual Rules
screenshots were compared with the reference at desktop and phone widths.

Syntax, seven Node checks, fresh-package smoke and 36 browser cases pass, including
native core commits, frozen-rule guards, save-bar positioning, app/budget payloads,
local discard and Rules navigation. The checks below retain their historical counts.

## History presentation checkpoint (2026-10-28, LuCI r8)

History now uses the reference's Screen time panel, range controls, daily bars,
summary cards and By app panel. The unchanged core API exposes seven daily totals
and today's top apps only. Month/year/all-time and earlier-week controls are disabled
with an explanation; historical budget lines and over-budget counts are unavailable,
not reconstructed from current configuration. Earlier per-app breakdowns and retention
controls require a future core API extension, not client-generated data.

Missing days are gaps, an explicitly reported zero is zero, and older positive values
with no coverage metadata remain marked as unknown coverage. Totals/averages exclude
missing or invalid samples. The app panel labels today's scope explicitly; selecting
a previous day never reuses today's app totals as that day's history. EN/DA, narrow
layouts and read-only native integration have browser coverage.

The person tab bar also no longer overflows vertically by one pixel: its negative
button margin was removed, vertical overflow is contained, and horizontal scrolling
and keyboard focus remain available. A targeted Chromium regression and local Firefox
probe confirm equal scroll/client heights. Fresh APK smoke, syntax, eight Node checks
and 39 browser cases pass for the combined History/tab-bar update. Desktop/mobile
screenshots were inspected with both real empty state and fictional test reports.

The checkpoints below record earlier milestones and their historical test counts.

## State-handling checkpoint (2026-10-25)

- Missing/malformed top-level reply envelopes fail the read rather than becoming
  an empty household. A legitimate empty response still clears the corresponding
  view. This does not claim complete validation of every future API field.
- Failed refreshes keep the last rendered data, label it as previously received
  with its last refresh time, and disable actions until a valid refresh succeeds.
- Mutations and reads are single-flight. Buttons/inputs are disabled during work;
  an additional programmatic click is also guarded. Core idempotency and
  authorization remain necessary regardless of those UI guards.
- Action errors have a separate live notice. A successful read after a refused or
  failed action cannot erase that error. A subsequent action clears the old notice.
- Polling does not repaint an input being edited. Amount drafts survive later
  refreshes and are removed when the request disappears from the pending set.
- `stop()` cancels polling, disables actions and ignores a late read response.
  The host owns the mount lifetime; disconnected roots do not schedule further
  refreshes after their current read completes.

## Evidence

Syntax checks and five fast Node tests pass. An immutable source snapshot passes
all **51 Playwright tests** against a disposable OpenWrt/LuCI container, including
real HTTP approval through the LuCI session shim. Five additional tests mount the
same production module in a neutral DOM host and verify:

1. Persistent refusal notices and exactly one in-flight decision.
2. Stale-data visibility, disabled actions and recovery to a valid empty state.
3. Initial malformed data is not an empty household, including Danish copy.
4. Focused editing and amount drafts survive polling.
5. A stopped mount ignores its late reply and does not restart polling.

A regression probe restores the previous module and confirms that the stale-data
case fails. Matching existing development packages were used with the changed
presentation source overlaid; this is source/runtime evidence, not a newly built
release package. The container was removed afterward. No production deployment.
No large design-board screenshots were loaded into the conversation.

## Usage coverage checkpoint (2026-10-26)

Core 0.10.0 adds `reported` to each administrator weekly day, distinguishing an
absent report from an authenticated zero count. It does not mean that all phones
reported or that their counters are complete/accurate. No retention, grant, budget
or signed-phone-policy semantics change.

The shared view renders missing days as gaps and measured zero as zero. With older
cores, absent coverage metadata remains explicitly unknown: positive counts can be
shown with a coverage warning, but ambiguous zero days are not labelled measured zero.
A profile without today's device reports displays a dash rather than a fabricated
zero. Each chart day has an EN/DA accessible label.

Go vet/race tests, six Node tests, syntax and all 53 real-LuCI/neutral browser tests
pass on an immutable final snapshot. The real HTTP fixture submits an authenticated
zero report and checks its week metadata through the session/ACL path. Its date comes
from the core's router-local day, not the fixture shell's timezone, and the assertion
uses the actual submitted date to avoid a midnight race. The browser tests cover
both EN and DA gaps, zero values and legacy coverage. A freshly compiled core binary
and changed UI were overlaid on matching development packages for runtime testing;
that local checkpoint did not claim new release-package installation. Follow-up
[CI 40279978074](https://github.com/denogio/familywarden/actions/runs/40279978074)
passes SDK/package smoke tests, aarch64 core build, packaged LuCI e2e and Android checks.
No production deployment or release.

## Still to implement

- The prototype's complete Family/child-detail, Devices, Administration and
  Ulysses presentation and navigation. Current module hardening is not that port.
- Real contracts for unsupported editors and device/health facts, with explicit
  loading, partial, unavailable and application states. Weekly presence metadata now
  exists; full history, complete coverage and deletion-state explanations still need
  their corresponding backend contracts.
- Authoritative configuration/pact ownership, CAS editing and safe apply/rollback
  in the core before replacing the corresponding legacy UCI writers.
- Neutral standalone **production** hosting/authentication, if chosen. The neutral
  test fixture proves presentation portability; it is not a standalone admin server.
- Visual/accessibility parity across the complete future interface and README
  screenshots once its implemented states are stable.

See [the prototype review](control-prototype-review.md),
[the core migration plan](../../plans/core-migration.md) and
[the current backlog](../../plans/app-control-todo.md).
