# Reboot usage continuity (RB1)

Verified against AOSP Android 15 (`android-15.0.0_r1`):

- [UsageEvents.Event](https://android.googlesource.com/platform/frameworks/base/+/android-15.0.0_r1/core/java/android/app/usage/UsageEvents.java):
  `DEVICE_SHUTDOWN` timestamps the last database persistence, not actual shutdown.
  `DEVICE_STARTUP` can also mean a runtime restart.
- [UserUsageStatsService](https://android.googlesource.com/platform/frameworks/base/+/android-15.0.0_r1/services/usage/java/com/android/server/usage/UserUsageStatsService.java):
  `init()` loads persisted stats and **synthesizes** shutdown/startup events from
  `lastTimeSaved`/`endTime` and current time. `reportEvent()` excludes shutdown events;
  `queryEvents()` reads the persisted/active event history. Events survive across boots,
  but the pair alone cannot distinguish a clean reboot from a crash or power loss.
- [Intent.ACTION_SHUTDOWN](https://android.googlesource.com/platform/frameworks/base/+/android-15.0.0_r1/core/java/android/content/Intent.java):
  protected system broadcast, runtime-registered receivers only since Android P.
  [ShutdownThread.run](https://android.googlesource.com/platform/frameworks/base/+/android-15.0.0_r1/services/core/java/com/android/server/power/ShutdownThread.java)
  sends it with `FLAG_RECEIVER_FOREGROUND | FLAG_RECEIVER_REGISTERED_ONLY` before reboot
  or shutdown, waiting at most `MAX_BROADCAST_TIME` (10 seconds).
- [UserManager](https://android.googlesource.com/platform/frameworks/base/+/android-15.0.0_r1/core/java/android/os/UserManager.java):
  `hasUserRestriction(DISALLOW_CONFIG_DATE_TIME)` checks the calling user's effective
  `no_config_date_time` restriction; DevicePolicyManager can set it via `addUserRestriction`.
  [DevicePolicyManagerService](https://android.googlesource.com/platform/frameworks/base/+/android-15.0.0_r1/services/devicepolicy/java/com/android/server/devicepolicy/DevicePolicyManagerService.java)
  `setAutoTimeEnabled()` writes `Settings.Global.AUTO_TIME`.

The app therefore uses its own atomic final checkpoint from the runtime shutdown
receiver, not the synthetic pair as proof. The receiver runs on the collector's worker;
queries remain capped at 50,000 events. Failure, missed broadcast, a stopped service,
missing/invalid marker, skipped boots, missing startup, invalid event order, backward
wall time or more than 24 hours of history all keep the strict gap path. Later samples
invalidate the marker. Old checkpoints without the additive marker remain supported.

`Mast.enforce` installs `no_config_date_time` only while paired as device owner;
it is not always in force. Require `Settings.Global.AUTO_TIME == 1` for continuity,
including when the restriction is present. This is a current clock-condition check,
not a historical audit of the settings while the process was stopped.

The final sample charges through its cursor. The interval until startup is off;
only events at/after startup can reopen visibility. The old elapsed-time anchor must
not advance across a boot. Grant clocks remain fail-closed until a fresh signed sync:
existing conservative missing-interval grant charging is unchanged, so grant spending
may include uncertain reboot overlap after re-anchoring; daily usage does not.
