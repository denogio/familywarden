package me.hojgaard.ulysses.mast

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
                if (intent.action == "me.hojgaard.ulysses.mast.DEBUG_PAIR") {
                    val info = Sync.parsePairingUri(intent.getStringExtra("uri") ?: "")
                    val store = Store(context)
                    store.unpair()
                    store.host = info.host
                    store.publicKey = info.publicKey
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
