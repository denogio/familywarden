# Installed-apps design — source-backed review

Reviewed 2026-10-29 after `fc3fa89`. Local export title:
**FamilyWarden Android · installed apps**.

Source: `FamilyWarden – Parent · Installed apps choose child, browse, search, set a rule, allowed hours (planned future).html`.
SHA-256: `4af99cdd276c9f8dd0a113d2250473e63aaf9d3e16fcc99a5a89e646e277c997`.

Read the complete exported text and inspected representative light/dark/200% screens
from the 27 phone mockups offline. Artifacts are in `/tmp/fw-installed-apps-design/`.
The hosted canvas and original export were not changed. This is not native runtime
or accessibility proof; the 200% mockups grow in height rather than demonstrating
scrolling in a fixed-height Android viewport.

## Use as the implementation direction

- Existing Family child selection → Controls → Installed apps; no second child wizard.
- Friendly labels and stable icon/letter tiles, search/filter, per-app details and
  reporting-phone provenance; never require parents to enter package IDs.
- Profile-wide rules across the child's phones, not an undisclosed per-device rule.
- Explicit stale/missing lists, protected system apps, retained drafts and unknown
  mutation outcomes. Keep the icon-only bottom navigation visible in the renderings.
- Allow / Daily limit / Always allowed / Block are existing product modes, but the
  native inventory and scoped writing channels are new integration work.
- **Allowed hours is future work**, including its editors and child opening-time text.
  No per-app schedule, precedence or capability change is authorized by a mockup.

## Reconcile before implementing

1. **Messages is not universally essential.** `AppEnforcer.essentialPackages()` protects
   calls/emergency, launcher, Settings, keyboard, active admins, FamilyWarden and alarms.
   Messaging is ordinary unless explicitly always allowed. Do not hard-code the mock's
   Messages → Essential row or confuse system-package flags with essential roles.
   Essential classification is phone-specific and observational, never a remotely
   attested permission to loosen policy. Mixed/unknown source classifications need
   truthful copy, while the local enforcer retains its unconditional safety checks.
2. **No new five-minute minimum.** Existing app quotas accept **0..1440 minutes**.
   The refusal example's “at least 5 minutes” is not today's contract. Zero quota is
   not interchangeable with explicit Block: grants can override a quota, not a block.
3. **Saved/received/applied are different.** Existing signed acknowledgements are
   observations at a fetch, not exact application timestamps or continuous enforcement.
   The saved screen cannot invent Received 17:46 / Applied 17:46 or assert that an
   offline tablet is still enforcing exactly 30 minutes. Use last acknowledged revision,
   pending confirmation and nullable observations until richer scoped APIs exist.
4. **Inventory is not usage or a pending-new-app list.** The child collector has an
   installed-package set locally. Uploads contain positive usage and at most 32 apps
   awaiting approval, not a complete installed snapshot, essential roles or phone icons.
   Never construct “All 24 installed apps” from those summaries. Empty, missing,
   partial/over-limit and failed inventory reads must differ.
5. **A generic failure is not a refusal.** Only an explicit pre-commit rejection can
   say nothing changed. Preserve the exact pending signed operation and draft through
   transport loss; Keep editing cannot erase an unresolved operation or trigger another
   save. Check status must reconcile saved state/revision before retrying. Preserve CAS
   conflicts and current authority if another parent edits in the meantime.
6. **Names do not uniquely identify apps.** Use package identity internally. Search
   reported friendly labels; distinguish duplicate names and label-less reports without
   asking users to type IDs. Do not invent categories or download icons from third-party
   services. Existing letter tiles are a valid first implementation, as in the export.
7. **Native size/interaction checks remain required.** The mock's growing 200% canvas
   does not prove scrolling, safe insets, minimum touch targets or reachable Save/Discard.
   Verify these in a fixed-height emulator, EN/DA, light/dark, with no visible nav labels.

## Authority and scope

The existing root `configuration_apply` path has core-owned validation, CAS and pact
protection. It is not a phone permission. The existing signed `app` operation handles
new-app approval/denial, not arbitrary standing-rule replacement. Add explicit bounded
phone contracts, recheck the current enrolled key/effective child scope and validate
inside the existing configuration authority. Administrator/captain flags must not
bypass parent scope; no phone-supplied method name may reach `/api/*`.

Execution and acceptance criteria: [installed-apps plan](../../plans/android-installed-apps.md).
