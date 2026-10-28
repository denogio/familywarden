# Shared FamilyWarden administration UI

Canonical platform-neutral UI sources live here, outside LuCI. There is no build
step, framework, CDN, router-specific business logic or standalone login.

- `control.js`: configuration, Family/profile controls and editors, Devices,
  Ulysses and Administration. It reuses `family.js` for Requests/usage/new apps.
- `control-i18n.js`: English and Danish resources.
- `control.css`: self-contained light/dark workspace styles, scoped under `.fwa`.
- `family.js`: the shared Requests/usage/new-app presentation, independently scoped under `.fwx`.

A host serves these files unchanged and calls `mount(root, options)`. It supplies
`call(method, params)` for the core API, locale, optional native enrollment facts
through `callHost`, and an optional protected-recovery URL. `mount` returns
`refresh()` and `stop()`; the host owns mount lifetime. UI code never imports LuCI,
UCI, ubus, PHP or a router SDK, and must not reimplement product eligibility.

The OpenWrt package installs these same sources under LuCI's public static-assets
URL. Only the LuCI view/host adapter contains session/ACL/RPC integration. A future
OPNsense host must reuse these sources and implement that small native host contract,
not fork or translate the presentation. No OPNsense implementation is claimed.

Neutral-host and real-LuCI tests use the same production modules. Core remains the
authority for every mutation, including frozen pacts, identity scopes, verified DNS
and confirmation rollback. Local drafts and UI guards are not authorization.
