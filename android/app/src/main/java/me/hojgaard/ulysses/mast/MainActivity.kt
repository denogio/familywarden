package me.hojgaard.ulysses.mast

import android.app.Activity
import android.app.AlertDialog
import android.content.Intent
import android.net.ConnectivityManager
import android.os.Bundle
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.ViewGroup.LayoutParams.WRAP_CONTENT
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import java.net.Inet4Address
import java.text.DateFormat
import java.util.Date

/** One screen: status, pairing, sync, and leaving (only without a pact). */
class MainActivity : Activity() {
    private lateinit var status: TextView
    private lateinit var scanButton: Button
    private lateinit var pairButton: Button
    private lateinit var releaseButton: Button

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val pad = (16 * resources.displayMetrics.density).toInt()
        status = TextView(this).apply { textSize = 16f }
        scanButton = Button(this).apply {
            text = getString(R.string.scan)
            setOnClickListener { scan() }
        }
        pairButton = Button(this).apply { setOnClickListener { pair() } }
        val syncButton = Button(this).apply {
            text = getString(R.string.sync_now)
            setOnClickListener { background({ Mast.refresh(this@MainActivity, sync = true) }) { render() } }
        }
        releaseButton = Button(this).apply {
            text = getString(R.string.release)
            setOnClickListener { release() }
        }
        val column = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(pad, pad, pad, pad)
            addView(TextView(context).apply { text = getString(R.string.title); textSize = 24f })
            addView(status, LinearLayout.LayoutParams(MATCH_PARENT, WRAP_CONTENT).apply { topMargin = pad })
            addView(syncButton)
            addView(scanButton)
            addView(pairButton)
            addView(releaseButton)
        }
        setContentView(ScrollView(this).apply { addView(column) })
        handlePairingLink(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handlePairingLink(intent)
    }

    private fun handlePairingLink(intent: Intent?) {
        val uri = intent?.data?.toString() ?: return
        if (uri.startsWith("ulyssesmast:")) confirmPairing(uri)
    }

    private fun scan() {
        val options = GmsBarcodeScannerOptions.Builder().setBarcodeFormats(Barcode.FORMAT_QR_CODE).build()
        GmsBarcodeScanning.getClient(this, options).startScan()
            .addOnSuccessListener { code -> code.rawValue?.let { confirmPairing(it) } }
            .addOnFailureListener { e -> message(getString(R.string.pair_failed, e.message)) }
    }

    /** The QR comes from LuCI (logged in), so its key is trusted; still show what is pinned. */
    private fun confirmPairing(uri: String) {
        if (!pairButton.isEnabled) return message(getString(R.string.pair_locked))
        val info = try {
            Sync.parsePairingUri(uri)
        } catch (e: Exception) {
            return message(getString(R.string.pair_failed, e.message))
        }
        AlertDialog.Builder(this)
            .setTitle(R.string.pair)
            .setMessage(getString(R.string.pair_confirm, "${info.host}\n${info.fingerprint}"))
            .setPositiveButton(android.R.string.ok) { _, _ -> store(info) }
            .setNegativeButton(android.R.string.cancel, null)
            .show()
    }

    private fun store(info: Sync.PairingInfo) {
        val store = Store(this)
        store.unpair()
        store.host = info.host
        store.publicKey = info.publicKey
        background({ Mast.refresh(this, sync = true) }) { render() }
    }

    private fun message(text: String) {
        AlertDialog.Builder(this).setMessage(text).setPositiveButton(android.R.string.ok, null).show()
    }

    override fun onResume() {
        super.onResume()
        background({ Mast.refresh(this, sync = true) }) { render() }
    }

    private fun render() {
        val store = Store(this)
        val state = store.state
        val decision = Mast.decide(this)
        val fmt = DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT)
        val lines = mutableListOf<String>()
        lines += if (Mast.isDeviceOwner(this)) getString(R.string.device_owner_yes) else getString(R.string.device_owner_no)
        if (store.paired) {
            lines += getString(R.string.paired_with, store.host, Usign.parsePublicKey(store.publicKey!!).fingerprint)
            lines += if (store.lastSync > 0) getString(R.string.last_sync, fmt.format(Date(store.lastSync * 1000))) else getString(R.string.never_synced)
            store.lastError?.let { lines += getString(R.string.last_error, it) }
            if (state != null && state.captain.isNotEmpty()) {
                lines += getString(R.string.captain, state.captain)
                lines += when {
                    !Policy.pactActive(state, System.currentTimeMillis() / 1000) -> getString(R.string.no_pact)
                    state.pactType == "ongoing" -> getString(R.string.pact_ongoing)
                    else -> getString(R.string.pact_until, fmt.format(Date(state.pactUntil * 1000)))
                }
                if (state.releaseAt > 0) lines += getString(R.string.release_at, fmt.format(Date(state.releaseAt * 1000)))
            }
        } else {
            lines += getString(R.string.not_paired)
        }
        lines += ""
        lines += if (decision.block) getString(R.string.hotspot_blocked) else getString(R.string.hotspot_allowed)
        status.text = lines.joinToString("\n")
        pairButton.text = getString(R.string.pair_manual)
        // Re-pairing could point the app at a router without a pact
        val bound = state != null && Policy.pactActive(state, System.currentTimeMillis() / 1000)
        pairButton.isEnabled = !bound
        scanButton.isEnabled = !bound
    }

    private fun pair() {
        val input = EditText(this).apply { setText(Store(this@MainActivity).host ?: defaultGateway() ?: "192.168.1.1") }
        AlertDialog.Builder(this)
            .setTitle(R.string.pair)
            .setMessage(R.string.pair_host)
            .setView(input)
            .setPositiveButton(android.R.string.ok) { _, _ ->
                val host = input.text.toString().trim()
                var info: Sync.PairingInfo? = null
                var error: String? = null
                background({ info = runCatching { Sync.fetchPairingInfo(host) }.onFailure { error = it.message }.getOrNull() }) {
                    val i = info
                    if (i == null) {
                        AlertDialog.Builder(this).setMessage(getString(R.string.pair_failed, error)).setPositiveButton(android.R.string.ok, null).show()
                        return@background
                    }
                    AlertDialog.Builder(this)
                        .setTitle(R.string.pair)
                        .setMessage(getString(R.string.pair_confirm, i.fingerprint))
                        .setPositiveButton(android.R.string.ok) { _, _ -> store(i) }
                        .setNegativeButton(android.R.string.cancel, null)
                        .show()
                }
            }
            .setNegativeButton(android.R.string.cancel, null)
            .show()
    }

    /** Leaving needs a fresh sync that confirms there is no pact — the cache isn't enough. */
    private fun release() {
        var allowed = false
        background({
            val store = Store(this)
            val fresh = !store.paired || Sync.fetch(store).let { r ->
                if (r is Sync.Result.Ok) {
                    store.payload = r.payload
                    store.lastSync = System.currentTimeMillis() / 1000
                    true
                } else false
            }
            allowed = fresh && Mast.canRelease(this)
        }) {
            if (!allowed) {
                AlertDialog.Builder(this).setMessage(R.string.release_refused).setPositiveButton(android.R.string.ok, null).show()
                return@background
            }
            AlertDialog.Builder(this)
                .setMessage(R.string.release_confirm)
                .setPositiveButton(android.R.string.ok) { _, _ -> Mast.release(this); render() }
                .setNegativeButton(android.R.string.cancel, null)
                .show()
        }
    }

    private fun defaultGateway(): String? {
        val cm = getSystemService(ConnectivityManager::class.java)
        val lp = cm.getLinkProperties(cm.activeNetwork) ?: return null
        return lp.routes.firstOrNull { it.isDefaultRoute && it.gateway is Inet4Address }?.gateway?.hostAddress
    }

    private fun background(work: () -> Unit, then: () -> Unit) {
        Thread {
            work()
            runOnUiThread { if (!isFinishing) then() }
        }.start()
    }
}
