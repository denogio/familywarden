# Shared control interface — implementation checkpoints

## Current boundary

The control prototype is a design reference, not a shipped whole-app interface.
The existing Requests page is the first real shared presentation slice:

- Presentation: `luci-app-familywarden/htdocs/luci-static/resources/familywarden/shared/family.js`.
- LuCI host adapter: `luci-app-familywarden/htdocs/luci-static/resources/view/familywarden/family.js`.
- Backend: existing core-backed `requests`, `usage`, `new_apps`, `request_answer`
  and `app_decision` RPC methods.

One plain ES module renders the page. The host supplies `call(method, params)`,
locale and optionally a clock/poll interval. The LuCI adapter keeps its normal
session and ACL path through rpcd to the core; no new password, port, iframe,
framework or CDN is introduced. Styles now live under `.fwa`; legacy `.fwx` hooks
remain on the same root/cards for existing tests and consumers, not a second UI.
The existing EN/DA tables remain complete for this slice.

The prototype's mock API shapes are **not** frozen by this work. Existing grant
limits, lifetime, requesting-device/key binding, parent scopes and pact exclusions
remain core decisions. The browser does not grant authority or infer enforcement.

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

## Still to implement

- The prototype's complete Family/child-detail, Devices, Administration and
  Ulysses presentation and navigation. Current module hardening is not that port.
- Real contracts for unsupported editors and device/health facts, with explicit
  loading, partial, unavailable and application states. Existing usage/weekly-chart
  semantics are unchanged here; truthful missing-day gaps need reported-day metadata
  from the core rather than treating an absent report as measured zero time.
- Authoritative configuration/pact ownership, CAS editing and safe apply/rollback
  in the core before replacing the corresponding legacy UCI writers.
- Neutral standalone **production** hosting/authentication, if chosen. The neutral
  test fixture proves presentation portability; it is not a standalone admin server.
- Visual/accessibility parity across the complete future interface and README
  screenshots once its implemented states are stable.

See [the prototype review](control-prototype-review.md),
[the core migration plan](../../plans/core-migration.md) and
[the current backlog](../../plans/app-control-todo.md).
