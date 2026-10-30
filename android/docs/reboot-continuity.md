# Reboot usage continuity (RB1)

Usage accounting is phone-owned. After an ordinary reboot, crash or raw reboot,
read the persisted Android UsageEvents history and resume from DEVICE_STARTUP.
Android can synthesize DEVICE_SHUTDOWN from its last persistence; it is not proof
of orderly shutdown. There is no ACTION_SHUTDOWN receiver or shutdown marker.
The powered-off interval is never foreground usage.

A successfully recovered boot deducts a fixed 15,000 ms from the current
router-local day's remaining total budget, once. Each previously checkpointed
visible ordinary app also receives a 15,000 ms budget debit. Essential and
always-allowed apps are excluded; multiple visible apps still debit the total
only once. An app need not currently have a limit to retain its debit.

The schema-1 journal stores `reboot_debits` separately from measured `days`, in
one atomic checkpoint with the boot cursor. Repeated samples and process restarts
on the same boot cannot debit again. Debit days share the journal's 30-day
retention. Legacy journals without debits still decode; obsolete shutdown objects
are ignored. Enforcement, remaining-time warnings and standing-limit grant
selection include debits. History and signed usage uploads use measured counters
only. Today's used labels also remain measured; remaining time includes the
adjustment. The debit itself never spends a signed grant.

Successful recovery neither increments `gaps` nor makes the day unknown. Existing
uncertainty from other problems is not cleared. Missing usage access, locked user,
unavailable/malformed history, missing startup, backward clock or history beyond
24 hours still fail closed. Same-boot elapsed regression is a gap, not a reboot
debit. AUTO_TIME is not reboot proof and is not required to read valid history;
same-boot wall/elapsed clock discontinuities remain gaps. Router unreachability
alone does not revoke local usage access.

Signed grant clocks remain unavailable until a fresh signed synchronization
anchors this boot. Existing conservative missing-interval grant accounting and
no-refill protections remain unchanged. The fixed 15-second compensation is a
product compromise, not a maximum persistence latency or a hard anti-reboot
no-free-time guarantee: Android can delay persistence longer than 15 seconds.
