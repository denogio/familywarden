package io.github.denogio.familywarden

/** Direct-boot-aware synthetic Home. Native manifest mode, not a boot callback, enters
 * lock task when the DPC has allowlisted this package. No credential storage is read
 * on ordinary launch. Only a disposable emulator may make this its default Home.
 */
class LockTaskHomeProbe : LockTaskProbeActivity()
