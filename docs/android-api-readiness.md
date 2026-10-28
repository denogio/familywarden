# Android API readiness

Source-backed inventory, 2026-10-28. This is an integration map, not permission to
extend a phone's role or expose the administrator socket on the LAN.

## Boundaries

- `core/internal/service/http.go`: `/api/*` is the host-authenticated administrator
  API. The OpenWrt host exposes it through LuCI/rpcd ACLs, not phone credentials.
- `/phone` authenticates enrolled phone keys and returns nonce-bound signed policy.
  `binding.approves` is the current parent scope, excluding a pact-bound captain.
- `/app/*` authenticates exact signed messages and rechecks the registry and current
  scope under the identity lock. Android actions fetch fresh policy before posting;
  mutation retries keep the exact original bytes in a private outbox.
- Neither administrator nor captain flags imply parent rights. Phone-read replies
  are information, never app policy, removal approval or permission to clear blocks.

## Existing contracts and remaining integrations

| Feature | Existing core path | Android / remaining boundary |
|---|---|---|
| Child app/total rules, downtime, bonus | Signed v3 `apps` | Parsed and enforced offline; do not create a second evaluator |
| Parent today's usage | Signed v3 `usage[]`, current `approves` scope | Daily total, apps and device reports; report receipt is not live device status |
| Retained app history | `UsageStore.UsageDay`, admin `usage` includes a week | Signed `history` read + `FamilyHistoryClient` prepared; one scoped day per read, no larger policy-sync payload |
| Extra time | Signed `request` / `decision` | Existing outbox, first answer wins, <= requested minutes, two-hour window |
| New-app decisions | Signed `app` | Existing scoped parent actions; installation can briefly precede suspension |
| Parent rule/DNS/network editing | `configuration_apply`, `configuration_bonus_add`, `configuration_block/allow/reset/ticket`, CAS/confirm | Administrator authority only today; no signed parent-edit permission/operation yet |
| Device applied state | Admin `devices`: expected revision vs acknowledgement | Not included in the parent's usage DTO; receipt/fetch cannot substitute |
| Ulysses reads | Signed `captain`, `pact`, `rules` | Existing self status; no invented voyage start or amendment metadata |
| Ulysses create/extend/amend | Admin `configuration_pact` / `configuration_amend`, core pact validation | No signed captain self-service mutation channel yet; do not forward root API |
| Protected release | Dedicated `/release/*`, native authentication | Router sail page only; no instant release in Android |
| Adult pool/categories | Planned product rules | No ledger/contract yet; not a frontend-only task |
| Domains / off-home delivery | Not implemented | No placeholder data or remote-availability claim |

## Implementation rules for this preparation

Reuse `dayUsage`, the enrolled-key verifier, exact-message reply binding, current
scope filtering and existing bounded HTTP transport. The implemented historical read requests one
profile and one router-local day (30-day retention), without writes, outbox, tickets,
policy acknowledgements or mutations. A missing day is absent, not zero usage.
Re-fetch scope for each read; removed roles, re-keyed phones and pact-protected
profiles must fail closed. Server errors must not masquerade as missing history.

Future parent editors need an explicit scoped core contract: allowed fields,
revision/CAS, validation, pact restrictions, native apply/confirm and retry semantics.
The existing root API is reusable product implementation, not reusable phone
permission. Do not implement those writes by passing a phone-supplied method name
to `/api/*`. The same restriction applies to Ulysses self-service and release.

## Prepared integration and verification

Core `0.13.0-r24` adds only the bounded signed `history` read and usage-report
receipt metadata. `FamilyHistoryClient.history(context, profileId, day)` must run
off the main thread. It performs a fresh nonce-bound policy preview to check scope
and the saved generation floor, then verifies the exact signed historical response.
It does not accept that preview as a policy, persist the historical read, change
an outbox, acknowledge a revision or open internet access. A missing day is typed
as `FamilyHistoryDay.usage == null`; refusal/transport/store failure throws instead
of becoming empty data. Existing policies, editors and release authority are unchanged.
The shared upload transport and reply verifier are reused, not duplicated.

Regression coverage: Go current-key/scope/pact protection, strict decoding, calendar
retention/DST, missing vs zero, storage failure, and production service/restart with
no durable generation advance; Android signed-query binding, stale/wrong/oversized
replies, receipt metadata, old-router omission, transport loss and actual HTTP;
shared inbox filters cover known expiry/offline/unknown clocks and revoked scopes.

Validation: full Go race/vet with the portable shared-UI proof; 212 JVM tests (zero
failures/errors/skips), lint/debug build, package smoke, and both focused UI scripts
including Danish 200%. The real Android/OpenWrt P8 flow passes 33 checkpoints with
fresh r24 packages, including historical read and scope revocation through native
CGI. One initial attempt failed at guest-to-router TCP bootstrap before tests;
a new isolated run passed. No network-root-cause fix is claimed. Disposable resources
were removed. Packages are local in `dist/android-api/`; no production deployment.

Design continuation: `docs/design/family-views/UI-SPEC.md`. Product decisions:
`plans/app-control.md`. Wire contract: `plans/app-control-protocol.md`.
