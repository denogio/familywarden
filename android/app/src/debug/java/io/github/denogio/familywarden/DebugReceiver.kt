package io.github.denogio.familywarden

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log

/**
 * Debug builds only (not in release): `am broadcast -a ...DEBUG_PAIR --es uri <pairing uri>`
 * and `...DEBUG_SYNC`, so the emulator test can drive the app without taps.
 */
class DebugReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val pending = goAsync()
        Thread {
            try {
                if (intent.action == "io.github.denogio.familywarden.DEBUG_PAIR") {
                    val info = Sync.parsePairingUri(intent.getStringExtra("uri") ?: "")
                    val address = Sync.numericEndpoint(info.host)
                    synchronized(Mast) {
                        val store = Store(context)
                        check(Mast.isDeviceOwner(context) && !store.provisioned) { "use the verified UI for an enrolled device" }
                        val reply = Sync.fetch(address, info.publicKey, store.deviceId, store.phoneKey, "Emulator")
                        check(reply is Sync.Result.Ok) { "unverified pairing reply" }
                        store.pairVerified(address, info.publicKey, reply)
                    }
                }
                Mast.refresh(context, sync = true)
                Log.i("UlyssesMast", "debug: decision=${Mast.decide(context)} error=${Store(context).lastError}")
            } catch (e: Exception) {
                Log.e("UlyssesMast", "debug: ${intent.action} failed", e)
            } finally {
                pending.finish()
            }
        }.start()
    }
}
