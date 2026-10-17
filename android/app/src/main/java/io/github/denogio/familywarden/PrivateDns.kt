package io.github.denogio.familywarden

import android.app.admin.DevicePolicyManager
import android.content.Context
import android.os.Looper
import android.os.UserManager

/**
 * Strict Private DNS from the verified policy: DNS-over-TLS to the profile's
 * AdGuard Home hostname, locked against changes. There is no unfiltered
 * fallback, but Android only accepts a hostname it can reach right now: until
 * that succeeds the earlier setting stays, and the app says so (dnsError).
 */
object PrivateDns {
    /** Apply verified DNS. Missing DNS is never permission to choose an unfiltered mode. */
    fun apply(context: Context, store: Store, dns: DnsPolicy?) {
        if (!Mast.isDeviceOwner(context)) return
        if (dns == null) {
            if (store.dnsOwned) store.dnsError = "DNS policy missing; settings retained"
            return
        }
        val dpm = Mast.dpm(context)
        val admin = Mast.admin(context)
        // From now on only FamilyWarden changes Private DNS
        dpm.addUserRestriction(admin, UserManager.DISALLOW_CONFIG_PRIVATE_DNS)
        store.dnsOwned = true
        if (readAppliedDns(context).hostname == dns.hostname) {
            store.dnsError = null
            return
        }
        // The setter validates the resolver over the network and may take seconds
        if (Looper.myLooper() == Looper.getMainLooper()) return
        store.dnsError = when (dpm.setGlobalPrivateDnsModeSpecifiedHost(admin, dns.hostname)) {
            DevicePolicyManager.PRIVATE_DNS_SET_NO_ERROR -> null
            DevicePolicyManager.PRIVATE_DNS_SET_ERROR_HOST_NOT_SERVING -> "resolver not serving"
            else -> "setting failed"
        }
    }

    /**
     * Give Private DNS back to the user: Automatic (opportunistic), unlocked.
     * Android can't restore an earlier "Off" exactly. false = not restored.
     */
    fun restore(context: Context, store: Store): Boolean {
        if (!store.dnsOwned || !Mast.isDeviceOwner(context)) return true
        val dpm = Mast.dpm(context)
        val admin = Mast.admin(context)
        if (dpm.setGlobalPrivateDnsModeOpportunistic(admin) != DevicePolicyManager.PRIVATE_DNS_SET_NO_ERROR) return false
        dpm.clearUserRestriction(admin, UserManager.DISALLOW_CONFIG_PRIVATE_DNS)
        store.dnsOwned = false
        store.dnsError = null
        return true
    }
}
