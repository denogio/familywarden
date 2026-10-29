package io.github.denogio.familywarden.ui

import androidx.compose.runtime.*
import androidx.compose.ui.tooling.preview.Preview
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.ui.theme.MastTheme
import java.time.*
import java.time.temporal.ChronoUnit

private const val epoch = 1800000000L
private val alex = ProfileRef("alex", "Alex")
private val sam = ProfileRef("sam", "Sam")
private val today = LocalDate.parse("2027-01-15")
private val family = FamilyWardenState(epoch * 1000, parentMode = true, paired = true, provisioned = true,
    timeZone = "UTC", requestEpoch = epoch, lastSync = epoch,
    binding = DeviceBinding("fw-parent", "parent", "Taylor", false, false, false, listOf(alex, sam)))

private fun previewHistory(profile: String, day: LocalDate): FamilyHistoryDay {
    val ref = if (profile == "alex") alex else sam
    val offset = ChronoUnit.DAYS.between(day, today)
    if (offset == 3L) throw HistoryRefused("unavailable")
    val usage = when (offset) {
        2L -> null
        1L -> FamilyUsage(ref, day.toString(), 0, listOf(DeviceUsage(ref.name + " phone", 0, false, epoch - 300)), emptyList())
        else -> FamilyUsage(ref, day.toString(), 82 * 60, listOf(DeviceUsage(ref.name + " phone", 82 * 60, offset > 3, epoch - 300)),
            listOf(AppSeconds("com.example.video", ref.name + " video", 30 * 60)))
    }
    return FamilyHistoryDay(ref, day.toString(), usage, epoch)
}
private fun previewApps(profile: String): InstalledApps {
    val ref = if (profile == "alex") alex else sam
    if (profile == "sam") return InstalledApps(ref, listOf(AppSource("Sam phone", false, null, null, null)),
        listOf(InstalledApp("com.example.chat", "", emptySet(), emptySet(), emptySet(), AppRule.Blocked)), "0123456789abcdef", true, epoch)
    fun app(pkg: String, label: String, rule: AppRule, on: Set<Int> = setOf(0, 1), system: Set<Int> = emptySet(),
        essential: Set<Int> = emptySet()) = InstalledApp(pkg, label, on, system, essential, rule)
    return InstalledApps(ref, listOf(AppSource("Alex phone", true, epoch - 1200, 9, 9),
        AppSource("Alex tablet", true, epoch - 3 * 86400, 7, 6)), listOf(
        app("com.android.dialer", "Phone", AppRule.None, system = setOf(0, 1), essential = setOf(0, 1)),
        app("com.example.chess", "Chess Club", AppRule.Allowed, setOf(0)),
        app("com.example.minecraft", "Minecraft", AppRule.Limit(60)),
        app("com.example.old", "", AppRule.Blocked, emptySet()),
        app("com.example.sketch", "Sketch Party", AppRule.None, setOf(1)),
        app("com.example.snapgram", "Snapgram", AppRule.Blocked, setOf(0)),
        app("com.example.spotify", "Spotify", AppRule.Always),
        app("com.example.video", "Video", AppRule.Limit(30)),
        app("com.example.video2", "Video", AppRule.None, setOf(1)),
    ), "0123456789abcdef", true, epoch)
}
private fun actions(sync: () -> Unit = {}) = FamilyWardenActions(sync, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {},
    loadHistory = ::previewHistory, loadApps = ::previewApps)

@Preview(name = "Parent — installed apps change between reads", widthDp = 400, heightDp = 840)
@Composable
private fun AppsChangePreview() {
    var state by remember { mutableStateOf(family) }
    var reads by remember { mutableIntStateOf(0) }
    val load = { profile: String -> previewApps(profile).let { data ->
        // First sync: Minecraft is blocked elsewhere. Second sync: the second Video app is removed.
        data.copy(apps = data.apps.filter { reads < 2 || it.pkg != "com.example.video2" }
            .map { if (reads >= 1 && it.pkg == "com.example.minecraft") it.copy(rule = AppRule.Blocked) else it })
    } }
    MastTheme { FamilyWardenScreen(state, FamilyWardenActions({ reads++; state = state.copy(lastSync = state.lastSync!! + 1) },
        {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, loadHistory = ::previewHistory, loadApps = load)) }
}

@Preview(name = "Parent — installed apps scope revoked", widthDp = 400, heightDp = 840)
@Composable
private fun AppsScopePreview() {
    var state by remember { mutableStateOf(family) }
    // Sam shares Alex's packages, so a stale selection would silently open Sam's editor.
    val load = { profile: String -> previewApps("alex").let { if (profile == "sam") it.copy(profile = sam) else it } }
    MastTheme { FamilyWardenScreen(state, FamilyWardenActions({ state = state.copy(binding = state.binding!!.copy(approves = listOf(sam))) },
        {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, loadHistory = ::previewHistory, loadApps = load)) }
}

@Preview(name = "Parent — working recorded-day history", widthDp = 400, heightDp = 840)
@Composable
private fun ParentHistoryPreview() { MastTheme { FamilyWardenScreen(family, actions()) } }

@Preview(name = "Parent — history scope revoked", widthDp = 400, heightDp = 840)
@Composable
private fun HistoryScopePreview() {
    var state by remember { mutableStateOf(family) }
    MastTheme { FamilyWardenScreen(state, actions { state = state.copy(binding = state.binding!!.copy(approves = listOf(alex))) }) }
}

private val voyage = family.copy(owner = true, parentMode = false, ulyssesEnabled = true,
    binding = DeviceBinding("fw-jordan", "jordan", "Jordan", true, false, false, listOf(alex)),
    pact = PactStatus.Bound(false, (epoch + 3600) * 1000, Pact(null, null, null)),
    rules = Rules(true, "allow", "none", emptyList()),
    voyageDetails = VoyageDetails(5400, Window(setOf(DayOfWeek.SUNDAY), LocalTime.of(10, 0), LocalTime.of(11, 0))))

@Preview(name = "Ulysses — ongoing voyage, hotspot allowed", widthDp = 400, heightDp = 840)
@Composable
private fun VoyageBoundPreview() { MastTheme { FamilyWardenScreen(voyage, actions()) } }

@Preview(name = "Ulysses — sea is calm", widthDp = 400, heightDp = 840)
@Composable
private fun VoyageCalmPreview() { MastTheme { FamilyWardenScreen(voyage.copy(pact = PactStatus.FreeSailing, voyageDetails = null), actions()) } }

@Preview(name = "Ulysses — release requested", widthDp = 400, heightDp = 840)
@Composable
private fun VoyageReleasePreview() { MastTheme { FamilyWardenScreen(voyage.copy(
    pact = PactStatus.Bound(true, null, Pact(null, (epoch + 5400) * 1000, null))), actions()) } }

@Preview(name = "Ulysses — exclusive midnight port call", widthDp = 400, heightDp = 840)
@Composable
private fun VoyageMidnightPreview() { MastTheme { FamilyWardenScreen(voyage.copy(voyageDetails =
    VoyageDetails(0, Window(setOf(DayOfWeek.SUNDAY), LocalTime.of(23, 0), LocalTime.MIDNIGHT))), actions()) } }

@Preview(name = "Ulysses — awaiting verified binding", widthDp = 400, heightDp = 840)
@Composable
private fun VoyagePendingPreview() { MastTheme { FamilyWardenScreen(voyage.copy(binding = null, pact = null), actions()) } }

@Preview(name = "Ulysses — no captain", widthDp = 400, heightDp = 840)
@Composable
private fun VoyageNoCaptainPreview() { MastTheme { FamilyWardenScreen(voyage.copy(pact = PactStatus.NoCaptain), actions()) } }

@Preview(name = "Ulysses — offline old router", widthDp = 400, heightDp = 840)
@Composable
private fun VoyageUnknownPreview() { MastTheme { FamilyWardenScreen(voyage.copy(
    syncError = "router unreachable", voyageDetails = null), actions()) } }
