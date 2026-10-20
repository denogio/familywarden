package io.github.denogio.familywarden

import android.app.admin.DevicePolicyManager
import android.content.Context
import android.os.UserManager

/** Read-back only. Policy receipt is not evidence that Android applied it. */
data class AppliedDns(val hostname: String? = null, val locked: Boolean = false)

fun readAppliedDns(context: Context): AppliedDns = runCatching {
    if (!Mast.isDeviceOwner(context)) return@runCatching AppliedDns()
    val dpm = Mast.dpm(context)
    val admin = Mast.admin(context)
    val specified = dpm.getGlobalPrivateDnsMode(admin) == DevicePolicyManager.PRIVATE_DNS_MODE_PROVIDER_HOSTNAME
    AppliedDns(
        hostname = if (specified) dpm.getGlobalPrivateDnsHost(admin)?.lowercase() else null,
        locked = dpm.getUserRestrictions(admin).getBoolean(UserManager.DISALLOW_CONFIG_PRIVATE_DNS),
    )
}.getOrDefault(AppliedDns())
