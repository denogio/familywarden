package io.github.denogio.familywarden

/** Signed per-device association. Display names never grant capabilities. */
data class DeviceBinding(
    val deviceId: String,
    val profileId: String,
    val profileName: String,
    val captain: Boolean,
    val administrator: Boolean,
    val selfRemoval: Boolean,
)

/** Policy IDs are typed/localized in the app; never render server-supplied HTML. */
data class DnsPolicy(val hostname: String, val protections: Set<Protection>, val services: List<String> = emptyList()) {
    init { require(validHostname(hostname)) { "invalid DNS hostname" } }
}

enum class Protection { FILTER_LISTS, SAFE_BROWSING, PARENTAL, SAFE_SEARCH }

/** DNS hostnames, not URLs or IP literals. Normalize only after validation. */
fun validHostname(hostname: String): Boolean =
    hostname.length in 3..253 && '.' in hostname &&
        hostname.any { it in 'a'..'z' || it in 'A'..'Z' } &&
        hostname.split('.').all { label ->
            label.length in 1..63 && label.first().isLetterOrDigit() && label.last().isLetterOrDigit() &&
                label.all { it in 'a'..'z' || it in 'A'..'Z' || it in '0'..'9' || it == '-' }
        }

/** No v1/global captain state may silently become this phone's authorization. */
fun MastState.appliesTo(deviceId: String): Boolean =
    wireVersion == 2 && !pending && binding?.deviceId == deviceId

/** Explicit signed authority plus no active applicable pact; "no pact" alone is not permission. */
fun MastState.canSelfRemove(deviceId: String, nowEpoch: Long): Boolean =
    appliesTo(deviceId) && binding?.selfRemoval == true &&
        !Policy.pactActive(this, nowEpoch)

/** For a newly verified request-bound reply, use the server clock, not a fast phone clock. */
fun Sync.Result.Ok.authorizesSelfRemoval(deviceId: String): Boolean =
    state.canSelfRemove(deviceId, state.serverTime)
