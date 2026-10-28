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
    private var uploadReply: UploadReply? = null
    private var nextParentSyncElapsed = 0L
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
        openUsageAccess = {
            runCatching { startActivity(Intent(android.provider.Settings.ACTION_USAGE_ACCESS_SETTINGS)) }
        },
        askTime = { target, minutes, note -> upload { AppRequestClient.request(this, target, minutes, note) } },
        answerTime = { request, approve, minutes -> upload { AppRequestClient.answer(this, request, approve, minutes) } },
        decideApp = { app, approve -> upload { AppRequestClient.decideApp(this, app, approve) } },
        retryUpload = { upload { AppRequestClient.retry(this) } },
        forgetUpload = { AppRequestClient.forgetRetry(this); uploadReply = null; render() },
        chooseParentMode = {
            if (!Mast.isDeviceOwner(this) && !Store(this).provisioned) {
                Store(this).parentMode = true
                if (android.os.Build.VERSION.SDK_INT >= 33) requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 77)
                render()
            }
        },
        parentSignOut = {
            if (!busy && !Mast.isDeviceOwner(this) && Store(this).parentMode)
                background({ Mast.unpairParent(this) }) { uploadReply = null }
        },
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        render()
        setContent { MastTheme { FamilyWardenScreen(ui, actions, initialRequests = intent.getBooleanExtra("requests", false)) } }
        getSystemService(ConnectivityManager::class.java).registerDefaultNetworkCallback(networkCallback)
        handlePairingLink(intent)
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.RESUMED) {
                while (true) {
                    render()
                    if (ui.paired && (ui.parentMode || ui.binding?.approves?.isNotEmpty() == true) && !busy &&
                        android.os.SystemClock.elapsedRealtime() >= nextParentSyncElapsed) refresh()
                    delay(1_000)
                }
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
        return (Mast.isDeviceOwner(this) || (store.parentMode && !store.provisioned)) && (!store.provisioned ||
            (store.effectiveState?.binding?.administrator == true &&
                store.effectiveState?.canSelfRemove(store.deviceId, System.currentTimeMillis() / 1000) == true))
    }

    /** An enrolled phone may always follow its own router (same pinned key) to a new address. */
    private fun canRepair(): Boolean = Store(this).let { it.provisioned && (Mast.isDeviceOwner(this) || it.parentMode) }

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
                    android.os.Build.MODEL.orEmpty(), requireDevicePolicy = store.provisioned,
                    capabilities = if (store.parentMode) "-" else PolicyCapabilities.advertised)
                check(result is Sync.Result.Ok)
                // A new pact or revoked authority must invalidate an already-open pairing sheet.
                if (store.provisioned) check(Mast.checkRelease(this))
                store.pairVerified(endpoint, info.publicKey, result)
                Mast.refresh(this, sync = false)
            }
        }) { if (it.isFailure) message(R.string.fw_error_pairing) }
    }

    private fun upload(work: () -> UploadReply) {
        if (busy) return
        uploadReply = null
        background(work) { result ->
            uploadReply = result.getOrNull()
            if (result.isFailure) message(R.string.fw_request_action_failed)
        }
    }

    private fun refresh() {
        nextParentSyncElapsed = android.os.SystemClock.elapsedRealtime() + 30_000
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
            parentMode = store.parentMode && !Mast.isDeviceOwner(this),
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
            childRequest = state?.request, parentRequests = state?.requests.orEmpty(),
            familyUsage = state?.familyUsage.orEmpty(), parentNewApps = state?.newApps.orEmpty(),
            waitingApps = if (state?.apps != null) UsageUpload.waitingApps(getSharedPreferences("usage-upload", MODE_PRIVATE))
                .map { (pkg, label) -> label.ifEmpty { pkg } }.sorted() else emptyList(),
            uploadPending = store.hasPendingUpload, uploadReply = uploadReply,
            canRequestTotal = state?.apps?.let { it.screenMinutes.isNotEmpty() || it.downtime.isNotEmpty() } == true,
            requestEpoch = state?.let { store.policyClock(it)?.now(android.os.SystemClock.elapsedRealtime(),
                android.provider.Settings.Global.getInt(contentResolver, android.provider.Settings.Global.BOOT_COUNT, -1)) }?.div(1000),
            requestTargets = state?.apps?.let { apps ->
                (apps.approved.intersect(AppControl.usageReport?.installed.orEmpty()) - apps.blocked - apps.alwaysAllowed - AppControl.usageReport?.essential.orEmpty()).map {
                    RequestTarget(it, AppControl.usageReport?.labels?.get(it) ?: it)
                }.sortedBy { it.label.lowercase() }
            }.orEmpty(),
            appsActive = state?.apps != null && Mast.isDeviceOwner(this), appsStatus = store.appsStatus,
            screenTime = state?.apps?.takeIf { Mast.isDeviceOwner(this) }?.let {
                screenTimeState(it, AppControl.usageReport, now.withZoneSameInstant(AppControl.zone(state)),
                    android.os.SystemClock.elapsedRealtime(), AppControl.hasUsageAccess(this))
            },
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
            |sync=${store.lastError ?: "ok"} lastSync=${store.lastSync} wire=${store.wireSeen}
            |apps=${store.effectiveState?.apps != null} appsStatus=${store.appsStatus ?: "none"} appsApplied=${store.appsApplied == store.effectiveState?.apps?.rev && store.appsApplied != null}
            |${usageToday(store)}
        """.trimMargin()
        getSystemService(ClipboardManager::class.java).setPrimaryClip(ClipData.newPlainText("FamilyWarden", report))
        message(R.string.fw_diagnostics_copied)
    }

    /**
     * Today's counted minutes for the apps the router limits: the only way to check counting
     * on a release build (not debuggable, no usage upload yet). Package names come from the
     * router's own policy; no other app activity is included.
     */
    private fun usageToday(store: Store): String {
        val state = store.effectiveState ?: return "usage=none"
        val apps = state.apps ?: return "usage=none"
        val zone = AppControl.zone(state)
        val counter = runCatching { UsageJournal(this).load() }.getOrNull()?.counter ?: return "usage=unavailable"
        val today = java.time.LocalDate.now(zone)
        val day = today.toString()
        val total = apps.toLocal(zone, today).dailyLimitMillis?.let { "%d/%dmin".format(java.util.Locale.ROOT, counter.used(day, UsageCounter.TOTAL) / 60_000, it / 60_000) }
            ?: "%dmin".format(java.util.Locale.ROOT, counter.used(day, UsageCounter.TOTAL) / 60_000)
        return "usage $day: total=$total " + apps.limitsMinutes.keys.sorted().joinToString(" ") { pkg ->
            "$pkg=%.1f/%dmin".format(java.util.Locale.ROOT, counter.used(day, pkg) / 60_000.0, apps.limitsMinutes.getValue(pkg))
        }.ifEmpty { "no limits" }
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
