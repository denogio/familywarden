package me.hojgaard.ulysses.mast

import android.content.Intent
import android.net.ConnectivityManager
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import me.hojgaard.ulysses.mast.ui.*
import me.hojgaard.ulysses.mast.ui.theme.MastTheme
import java.net.Inet4Address
import java.time.ZonedDateTime

/** Compose renders state; pairing, signed sync and device-owner actions stay here. */
class MainActivity : ComponentActivity() {
    private var ui by mutableStateOf(MastUiState(System.currentTimeMillis(), SetupState.NotPaired))
    private var dialog: MastDialog? = null
    private var pendingPairing: Sync.PairingInfo? = null
    private var busy = false

    private val actions = MastActions(
        onSync = { refresh() },
        onCheckDeviceOwner = { render() },
        onScanPairingCode = { scan() },
        onPairManually = {
            if (!bound()) {
                dialog = MastDialog.ManualAddress(Store(this).host ?: defaultGateway() ?: "192.168.1.1")
                render()
            }
        },
        onSubmitAddress = { host -> fetchPairing(host) },
        onConfirmPairing = { pendingPairing?.let { store(it) } },
        onRemove = { release(confirm = false) },
        onConfirmRemove = { release(confirm = true) },
        onDismissDialog = { dialog = null; pendingPairing = null; render() },
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        render()
        setContent { MastTheme { MastScreen(ui, actions) } }
        handlePairingLink(intent)
        // Re-read the cache after worker/alarm updates, and expire finite pacts locally.
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.RESUMED) {
                while (true) {
                    render()
                    delay(1_000)
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handlePairingLink(intent)
    }

    override fun onResume() {
        super.onResume()
        render()
        refresh()
    }

    private fun handlePairingLink(intent: Intent?) {
        intent?.data?.toString()?.takeIf { it.startsWith("ulyssesmast:") }?.let(::confirmPairing)
    }

    private fun bound(): Boolean = Store(this).state?.let {
        Policy.pactActive(it, System.currentTimeMillis() / 1000)
    } ?: false

    private fun scan() {
        if (bound() || busy) return
        val options = GmsBarcodeScannerOptions.Builder().setBarcodeFormats(Barcode.FORMAT_QR_CODE).build()
        GmsBarcodeScanning.getClient(this, options).startScan()
            .addOnSuccessListener { code -> code.rawValue?.let(::confirmPairing) }
            .addOnFailureListener { e -> message(getString(R.string.pair_failed, e.message)) }
    }

    private fun confirmPairing(uri: String) {
        if (bound()) return message(getString(R.string.pair_locked))
        val info = try {
            Sync.parsePairingUri(uri)
        } catch (e: Exception) {
            return message(getString(R.string.pair_failed, e.message))
        }
        showPairing(info)
    }

    private fun showPairing(info: Sync.PairingInfo) {
        if (bound()) return
        pendingPairing = info
        dialog = MastDialog.ConfirmPairing(info.host, info.fingerprint)
        render()
    }

    private fun fetchPairing(host: String) {
        if (bound() || busy) return
        background({ Sync.fetchPairingInfo(host) }) { result ->
            result.fold(::showPairing) {
                dialog = MastDialog.ManualAddress(host, it.message)
                render()
            }
        }
    }

    private fun store(info: Sync.PairingInfo) {
        if (busy) return
        dialog = null
        pendingPairing = null
        background({
            // A background sync may have bound us while the confirmation sheet was open.
            synchronized(Mast) {
                check(!bound()) { getString(R.string.pair_locked) }
                Store(this).apply {
                    unpair()
                    host = info.host
                    publicKey = info.publicKey
                }
                Mast.refresh(this, sync = true)
            }
        }) { result -> result.exceptionOrNull()?.let { message(it.message.orEmpty()) } }
    }

    private fun refresh() {
        if (busy) return
        background({ Mast.refresh(this, sync = true) }) { result ->
            result.exceptionOrNull()?.let { message(it.message.orEmpty()) }
        }
    }

    private fun render() {
        val store = Store(this)
        val state = store.state
        val now = ZonedDateTime.now()
        ui = mapMastUiState(
            nowMillis = now.toInstant().toEpochMilli(),
            isDeviceOwner = Mast.isDeviceOwner(this),
            routerHost = store.host,
            fingerprint = store.publicKey?.let { Usign.parsePublicKey(it).fingerprint },
            mastState = state,
            decision = Policy.decide(state, now),
            nextChangeEpoch = Policy.nextBlockChange(state, now)?.toEpochSecond(),
            lastSyncEpoch = store.lastSync,
            lastError = store.lastError,
            isSyncing = busy,
            dialog = dialog,
        )
    }

    /** Both opening AND confirming removal require a fresh, signed no-pact response. */
    private fun release(confirm: Boolean) {
        if (busy) return
        dialog = null
        background({
            synchronized(Mast) {
                val store = Store(this)
                val fresh = !store.paired || when (val result = Sync.fetch(store)) {
                    is Sync.Result.Ok -> {
                        store.payload = result.payload
                        store.lastSync = System.currentTimeMillis() / 1000
                        store.lastError = null
                        true
                    }
                    is Sync.Result.Failed -> {
                        store.lastError = result.reason
                        false
                    }
                }
                val allowed = fresh && Mast.canRelease(this)
                if (allowed && confirm) Mast.release(this)
                else Mast.refresh(this, sync = false)
                allowed
            }
        }) { result ->
            dialog = when {
                result.getOrDefault(false) && confirm -> null
                result.getOrDefault(false) -> MastDialog.ConfirmRemove
                else -> MastDialog.RemoveRefused
            }
        }
    }

    private fun message(text: String) = Toast.makeText(this, text, Toast.LENGTH_LONG).show()

    private fun defaultGateway(): String? {
        val cm = getSystemService(ConnectivityManager::class.java)
        val lp = cm.getLinkProperties(cm.activeNetwork) ?: return null
        return lp.routes.firstOrNull { it.isDefaultRoute && it.gateway is Inet4Address }?.gateway?.hostAddress
    }

    private fun <T> background(work: () -> T, then: (Result<T>) -> Unit) {
        busy = true
        render()
        lifecycleScope.launch {
            val result = withContext(Dispatchers.IO) { runCatching(work) }
            busy = false
            then(result)
            render()
        }
    }
}
