# Plan: Align FamilyWarden with nftables Kernel-Level Scheduling

## Context

The current architecture uses a cron-driven daemon that polls every minute to evaluate schedules in user-space. This is fragile:
- Clock drift, DST, timezone changes cause incorrect evaluation
- Race conditions on overlapping schedules
- Reload storms when many profiles change
- Wastes CPU polling when the kernel can do this natively

The new architecture (from `new-thoughts.md`) moves schedule enforcement into the kernel using nftables `meta hour` and `meta day` matching. The kernel evaluates time conditions at packet processing time — no polling, no daemon.

## Target Architecture

```
Save & Apply → UCI /etc/config/familywarden
                    ↓
         procd_add_reload_trigger fires
                    ↓
         /etc/init.d/familywarden reload
                    ↓
         /usr/libexec/rpcd/familywarden reload
                    ↓
         Generate /var/etc/familywarden.include (nftables rules)
                    ↓
         fw4 reload (applies include to kernel nftables)
                    ↓
         Kernel enforces schedules via meta hour/day rules
```

**Overrides (block/allow/ticket)** still go through ubus → `/tmp/familywarden.state` (RAM only, no flash writes). These are evaluated at `fw4 reload` time and injected as unconditional rules with higher priority than scheduled rules.

---

## Changes Required

### 1. Rewrite Init Script → procd with reload trigger

**File:** `familywarden/etc/init.d/familywarden`

Convert from classic rc.common to procd. Key changes:
- Use `USE_PROCD=1` and `/etc/rc.common`
- Add `service_triggers()` with `procd_add_reload_trigger "familywarden"`
- `reload_service` calls `/usr/libexec/rpcd/familywarden reload`
- Remove cron job installation entirely (no more `eval` every minute)
- `start_service` calls `fw4 reload` to apply initial rules

### 2. Rewrite rpcd Backend to Generate nftables Include

**File:** `familywarden/usr/lib/rpcd/familywarden`

Transform from pass-through wrapper to generator. The `reload` method must:
1. Read all profiles + schedules from UCI
2. Read transient overrides from `/tmp/familywarden.state`
3. Generate `/var/etc/familywarden.include` with complete nftables ruleset:
   - Base chain definition (hook + policy)
   - Per-profile rules with `meta hour`/`meta day` matching for scheduled profiles
   - Override rules (block/allow) injected after scheduled rules with higher priority
4. Call `fw4 reload` to apply

**nftables rule structure for a scheduled profile:**
```
# For profile "alex" with schedule Mon-Fri 20:00-06:00 (overnight):
table inet fw4 {
    chain familywarden_forward {
        type filter hook forward priority 0; policy accept;

        # Scheduled block (weekday evenings + night)
        ether saddr 00:11:22:33:44:55 ether daddr 00:11:22:33:44:55 \
            meta hour >= 20 meta day {1,2,3,4,5} \
            counter drop comment "familywarden:alex:scheduled"

        # Or for daytime schedule 07:00-20:00:
        ether saddr 00:11:22:33:44:55 ether daddr 00:11:22:33:44:55 \
            meta hour >= 7 meta hour < 20 meta day {1,2,3,4,5} \
            counter drop comment "familywarden:alex:scheduled"
    }
}
```

**Override rules** (block/allow/ticket) are inserted with a separate chain or with `goto` to take precedence over scheduled rules.

### 3. Firewall Include in fw4

**File:** `familywarden/etc/config/familywarden` (add to package) + runtime injection

The package must add this to `/etc/config/firewall`:
```
config include
    option name 'familywarden'
    option fw4_compatible '1'
    option path '/var/etc/familywarden.include'
```

This makes fw4 automatically include our generated rules on every `fw4 reload`.

### 4. Eliminate Cron/Polling

**Remove from init script:**
- No more cron job `* * * * * /usr/bin/familywarden eval`
- No more `/etc/init.d/cron start`

**Changes to `familywarden/usr/bin/familywarden.sh`:**
- Keep `init` command for initial setup
- Keep `block`, `allow`, `reset`, `ticket` commands (write to `/tmp/familywarden.state`)
- Remove `eval` command (no more cron-driven evaluation)
- Remove schedule evaluation logic (`is_schedule_active`, `has_active_schedule`)
- Remove direct `nft add rule` / `nft delete rule` calls (replaced by include file generation)
- The script becomes a stateless command tool, not a daemon

### 5. Keep Transient State in RAM

**File:** `/tmp/familywarden.state` — remains as-is

Overrides (block/allow/ticket) are stored in RAM only:
```
alex.override=block
alex.override_expires=
levi.override=allow
levi.override_expires=1751234567
```

On `reload`, the rpcd backend reads this file and generates override rules with higher priority than scheduled rules.

### 6. Fix Delete/Add Profile (already done)

The `uci.commit()` fix from the previous session is correct — bypasses rollback window. Keep it.

---

## Files to Modify

| File | Change |
|------|--------|
| `familywarden/etc/init.d/familywarden` | Rewrite as procd service with reload trigger |
| `familywarden/usr/lib/rpcd/familywarden` | Rewrite `reload` to generate nftables include |
| `familywarden/usr/bin/familywarden.sh` | Strip daemon logic, keep state helpers + ubus commands |
| `familywarden/Makefile` | Add firewall include installation |
| `familywarden/etc/config/familywarden` | Package ships a default config with the firewall include option |
| `luci-app-familywarden/htdocs/.../profiles.js` | Keep `uci.commit()` fix |

---

## nftables meta hour/day Syntax

```sh
# Monday-Friday, 20:00-06:00 (overnight block)
meta hour >= 20 meta day {1,2,3,4,5}
meta hour < 6 meta day {1,2,3,4,5}

# Monday-Friday, 07:00-20:00 (day block)
meta hour >= 7 meta hour < 20 meta day {1,2,3,4,5}

# Days: 0=Sunday, 1=Monday, ..., 6=Saturday
# Hours: 0-23
# Combine with "or" for overnight: hour >= 20 or hour < 6
```

---

## Verification

1. **Build packages** — no syntax errors
2. **Deploy to QEMU** — install both apks
3. **Check nftables include generated:**
   ```sh
   /usr/libexec/rpcd/familywarden reload
   cat /var/etc/familywarden.include
   ```
4. **Verify rules in kernel:**
   ```sh
   nft list chain inet fw4 familywarden_forward
   ```
5. **Test schedule enforcement** — set a schedule for current time, verify packets are dropped
6. **Test override** — `ubus call familywarden block '{"profile":"alex"}'` should insert a higher-priority rule
7. **Test delete profile** — delete via LuCI, verify nftables rules are gone after reload
8. **Test ticket** — `ubus call familywarden ticket '{"profile":"alex","minutes":30}'`, verify override rule appears

---

## Notes

- The `meta hour` and `meta day` matching requires kernel support — all OpenWrt 23.x+ kernels support this
- The `familywarden_forward` chain should use `priority 0` (same as fw4 forwarding) or `-1` to run before other rules
- Override rules (block/allow) must have higher priority (lower priority number) than scheduled rules, or use `goto` to jump to a separate chain
- The `fw4 reload` command is expensive — it reloads all firewall rules. For override-only changes (block/allow/ticket), consider using direct `nft` commands to insert/delete individual rules without full fw4 reload
