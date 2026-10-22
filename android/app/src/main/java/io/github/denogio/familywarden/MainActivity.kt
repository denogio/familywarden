package io.github.denogio.familywarden

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Intent
import android.net.ConnectivityManager
import android.net.Network
import android.net.Uri
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
import io.github.denogio.familywarden.ui.*
import io.github.denogio.familywarden.ui.theme.MastTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.net.Inet4Address

/** Compose renders facts; trusted policy and owner actions remain outside the UI. */
class MainActivity : ComponentActivity() {
    private var ui by mutableStateOf(FamilyWardenState(System.currentTimeMillis()))
    private var dialog: MastDialog? = null
    private var pendingPairing: Sync.PairingInfo? = null
    private var busy = false
    // A resolver test only describes the network (and policy) it ran on. Every
    // default-network change starts a new generation, so an old result reads as
    // "not tested" even after returning to the same network.
    private var resolverCheck: ResolverCheck? = null
    @Volatile private var generation = 0L
    private val networkCallback = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(network: Network) { generation++; runOnUiThread { render() } }
        override fun onLost(network: Network) { generation++; runOnUiThread { render() } }
    }

    private val actions = FamilyWardenActions(
        sync = { refresh() }, scan = { scan() },
        manualPair = {
            if (canPair() || canRepair()) {
                dialog = MastDialog.ManualAddress(Store(this).host ?: defaultGateway() ?: "192.168.1.1")
                render()
            }
        },
        submitAddress = { fetchPairing(it) },
        confirmPair = { pendingPairing?.let(::pair) },
        dismiss = { dialog = null; pendingPairing = null; render() },
        remove = { release(false) }, confirmRemove = { release(true) },
        toggleUlysses = { enabled ->
            synchronized(Mast) {
                val store = Store(this)
                val state = store.effectiveState
                if (state?.binding?.captain == true && !Policy.pactActive(state, System.currentTimeMillis() / 1000)) {
                    store.ulyssesEnabled = enabled
                }
            }
            render()
        },
        testResolver = { testResolver() },
        openSail = {
            Store(this).host?.let { host ->
                runCatching { startActivity(Intent(Intent.ACTION_VIEW, Uri.parse("${Sync.base(host)}/"))) }
                    .onFailure { message(R.string.fw_router_offline) }
            }
        },
        copyDiagnostics = { copyDiagnostics() },
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        render()
        setContent { MastTheme { FamilyWardenScreen(ui, actions) } }
        getSystemService(ConnectivityManager::class.java).registerDefaultNetworkCallback(networkCallback)
        handlePairingLink(intent)
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.RESUMED) {
                while (true) { render(); delay(1_000) }
            }
        }
    }

    override fun onDestroy() {
        getSystemService(ConnectivityManager::class.java).unregisterNetworkCallback(networkCallback)
        super.onDestroy()
    }

    /** One DoT query to the applied resolver, bound to the current network and policy. */
    private fun testResolver() {
        val policy = Store(this).effectiveState?.dns ?: return
        val cm = getSystemService(ConnectivityManager::class.java)
        val network = cm.activeNetwork ?: return
        val gen = generation
        val started = ResolverCheck(policy, network.networkHandle, System.currentTimeMillis(), ProbeResult.TESTING, gen)
        resolverCheck = started
        render()
        lifecycleScope.launch {
            val result = withContext(Dispatchers.IO) { DotProbe.probe(network, policy.hostname) }
            if (resolverCheck === started) resolverCheck = started.copy(testedAt = System.currentTimeMillis(), result = result)
            render()
        }
    }

    override fun onNewIntent(intent: Intent) { super.onNewIntent(intent); handlePairingLink(intent) }
    override fun onResume() { super.onResume(); render(); refresh() }

    private fun handlePairingLink(intent: Intent?) {
        val link = intent?.data ?: return
        if (link.scheme in setOf("familywarden", "ulyssesmast")) {
            intent.data = null // Do not reopen a consumed pairing link after locale/rotation recreation.
            confirmPairing(link.toString())
        }
    }

    private fun canPair(): Boolean {
        val store = Store(this)
        return Mast.isDeviceOwner(this) && (!store.provisioned ||
            (store.effectiveState?.binding?.administrator == true &&
                store.effectiveState?.canSelfRemove(store.deviceId, System.currentTimeMillis() / 1000) == true))
    }

    /** An enrolled phone may always follow its own router (same pinned key) to a new address. */
    private fun canRepair(): Boolean = Mast.isDeviceOwner(this) && Store(this).provisioned

    private fun scan() {
        if (!(canPair() || canRepair()) || busy) return
        val options = GmsBarcodeScannerOptions.Builder().setBarcodeFormats(Barcode.FORMAT_QR_CODE).build()
        GmsBarcodeScanning.getClient(this, options).startScan()
            .addOnSuccessListener { it.rawValue?.let(::confirmPairing) }
            .addOnFailureListener { message(R.string.fw_error_pairing) }
    }

    private fun confirmPairing(uri: String) {
        if (!(canPair() || canRepair())) return message(R.string.pair_locked)
        runCatching { Sync.parsePairingUri(uri) }.fold(::showPairing) { message(R.string.fw_error_pairing) }
    }

    private fun showPairing(info: Sync.PairingInfo) {
        val store = Store(this)
        if (Sync.isAddressRepair(store.provisioned, store.publicKey, info)) return repair(info)
        if (!canPair()) return message(R.string.pair_locked)
        pendingPairing = info
        dialog = MastDialog.ConfirmPairing(info.host, info.fingerprint)
        render()
    }

    private fun fetchPairing(host: String) {
        if (!(canPair() || canRepair()) || busy) return
        background({ Sync.fetchPairingInfo(host) }) { result ->
            result.fold(::showPairing) {
                dialog = MastDialog.ManualAddress(host, getString(R.string.fw_error_pairing))
            }
        }
    }

    /** Same key, new address: no fingerprint dialog, the pin itself is the check. */
    private fun repair(info: Sync.PairingInfo) {
        if (busy) return
        dialog = null
        background({ Mast.repairAddress(this, Sync.numericEndpoint(info.host)) }) {
            message(if (it.getOrDefault(false)) R.string.fw_address_repaired else R.string.fw_error_pairing)
        }
    }

    private fun pair(info: Sync.PairingInfo) {
        if (busy) return
        dialog = null
        pendingPairing = null
        background({
            // Resolve legacy/manual hostnames once, BEFORE touching the old pairing/cache.
            val endpoint = Sync.numericEndpoint(info.host)
            synchronized(Mast) {
                check(canPair())
                val store = Store(this)
                val result = Sync.fetch(endpoint, info.publicKey, store.deviceId, store.phoneKey,
                    android.os.Build.MODEL.orEmpty(), requireDevicePolicy = store.provisioned)
                check(result is Sync.Result.Ok)
                // A new pact or revoked authority must invalidate an already-open pairing sheet.
                if (store.provisioned) check(Mast.checkRelease(this))
                store.pairVerified(endpoint, info.publicKey, result)
                Mast.refresh(this, sync = false)
            }
        }) { if (it.isFailure) message(R.string.fw_error_pairing) }
    }

    private fun refresh() {
        if (!busy) background({ Mast.refresh(this, sync = true) }) {
            if (it.isFailure) message(R.string.fw_sync_failed)
        }
    }

    private fun render() {
        val store = Store(this)
        val raw = store.state
        val state = raw?.takeIf { it.appliesTo(store.deviceId) }
        val now = Mast.now(state)
        val applied = readAppliedDns(this)
        val mapped = mapMastUiState(
            nowMillis = now.toInstant().toEpochMilli(), isDeviceOwner = Mast.isDeviceOwner(this),
            routerHost = store.host,
            fingerprint = store.publicKey?.let { runCatching { Usign.parsePublicKey(it).fingerprint }.getOrNull() },
            mastState = state, decision = Policy.decide(state, now),
            nextChangeEpoch = Policy.nextBlockChange(state, now)?.toEpochSecond(),
            lastSyncEpoch = store.lastSync, lastError = store.lastError,
        )
        ui = FamilyWardenState(
            nowMillis = now.toInstant().toEpochMilli(), owner = Mast.isDeviceOwner(this),
            paired = store.paired, provisioned = store.provisioned, binding = state?.binding,
            policyUnsupported = raw?.wireVersion == 1,
            awaitingApproval = store.awaitingApproval,
            phoneFingerprint = store.phoneKey.fingerprint,
            resolverCheck = resolverCheck, probeGeneration = generation,
            canTestResolver = Mast.isDeviceOwner(this) && state?.dns != null &&
                applied.hostname == state.dns.hostname && applied.locked,
            host = store.host, fingerprint = mapped.details.fingerprint,
            lastSync = store.lastSync, syncError = store.lastError, syncing = busy,
            ulyssesEnabled = store.ulyssesEnabled, pact = (mapped.setup as? SetupState.Paired)?.status,
            rules = state?.rules, timeZone = state?.timeZone, desiredDns = state?.dns,
            appliedDnsHost = applied.hostname, dnsLocked = applied.locked, dnsError = store.dnsError,
            networkId = getSystemService(ConnectivityManager::class.java).activeNetwork?.networkHandle,
            dialog = dialog,
        )
    }

    /** Opening the sheet and final removal both require current authorization. */
    private fun release(confirm: Boolean) {
        if (busy) return
        dialog = null
        background({
            if (confirm) Mast.release(this) else Mast.checkRelease(this)
        }) { result ->
            dialog = when {
                !result.getOrDefault(false) -> MastDialog.RemoveRefused(
                    if (result.isFailure) RemovalRefusal.NotCompleted
                    else Store(this).let { removalRefusal(it.effectiveState, it.lastError) })
                confirm -> null
                else -> MastDialog.ConfirmRemove
            }
        }
    }

    private fun copyDiagnostics() {
        val store = Store(this)
        val applied = readAppliedDns(this)
        // Deliberately omit names, ClientIDs, addresses, keys, and payload contents.
        val report = """FamilyWarden ${BuildConfig.VERSION_NAME}
            |owner=${Mast.isDeviceOwner(this)} paired=${store.paired} provisioned=${store.provisioned}
            |devicePolicy=${store.effectiveState != null} awaitingApproval=${store.awaitingApproval} dnsPolicy=${store.effectiveState?.dns != null}
            |dnsApplyError=${store.dnsError ?: "none"} dnsOwned=${store.dnsOwned}
            |dnsMatches=${applied.hostname != null && applied.hostname == store.effectiveState?.dns?.hostname} dnsLocked=${applied.locked}
            |sync=${store.lastError ?: "ok"} lastSync=${store.lastSync}
        """.trimMargin()
        getSystemService(ClipboardManager::class.java).setPrimaryClip(ClipData.newPlainText("FamilyWarden", report))
        message(R.string.fw_diagnostics_copied)
    }

    private fun message(id: Int) = Toast.makeText(this, id, Toast.LENGTH_LONG).show()

    private fun defaultGateway(): String? {
        val cm = getSystemService(ConnectivityManager::class.java)
        return cm.getLinkProperties(cm.activeNetwork)?.routes?.firstOrNull {
            it.isDefaultRoute && it.gateway is Inet4Address
        }?.gateway?.hostAddress
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
