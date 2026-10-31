package io.github.denogio.familywarden.ui

import android.content.res.Configuration
import androidx.compose.runtime.Composable
import androidx.compose.ui.tooling.preview.Preview
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.ui.theme.MastTheme

private val previewActions = FamilyWardenActions({}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {})
private val previewDns = DnsPolicy("alex.dns.example.net", setOf(Protection.FILTER_LISTS, Protection.SAFE_SEARCH))
private val previewNow = 1_800_000_000_000L
private val previewState = FamilyWardenState(
    nowMillis = previewNow, owner = true, paired = true, provisioned = true,
    binding = DeviceBinding("fw-example", "captain", "Alex", true, true, true),
    host = "192.0.2.1", lastSync = previewNow / 1000, desiredDns = previewDns,
    appliedDnsHost = previewDns.hostname, dnsLocked = true, networkId = 1,
    resolverCheck = ResolverCheck(previewDns, 1, previewNow, ProbeResult.ANSWERED),
)

private val screenNow = java.time.ZonedDateTime.parse("2026-10-23T19:00:00+02:00[Europe/Copenhagen]")
private val screenApps = AppsPolicy("0123456789abcdef", false,
    mapOf("com.example.video" to 30, "com.example.game" to 60), emptySet(), emptySet(), emptyList(),
    mapOf(screenNow.dayOfWeek to 120), setOf("com.example.school"))
private val screenReport = AppUsageReport(screenApps.rev, screenNow.zone.id, screenNow.toLocalDate(), 1000,
    true, mapOf(UsageCounter.TOTAL to 82 * 60_000L, "com.example.video" to 26 * 60_000L,
        "com.example.game" to 45 * 60_000L),
    mapOf("com.example.video" to "Video", "com.example.game" to "Game", "com.example.school" to "School"),
    mapOf(screenNow.toLocalDate().toString() to 82 * 60_000L,
        screenNow.toLocalDate().minusDays(1).toString() to 45 * 60_000L),
    historyApps = mapOf(screenNow.toLocalDate().toString() to mapOf("com.example.game" to 45 * 60_000L,
        "com.example.video" to 12 * 60_000L)))
private val screenState = previewState.copy(nowMillis = screenNow.toInstant().toEpochMilli(),
    lastSync = screenNow.toEpochSecond(),
    binding = previewState.binding!!.copy(captain = false, administrator = false),
    appsActive = true, appsStatus = "ok", screenTime = screenTimeState(screenApps, screenReport, screenNow, 1000, true))

@Preview(name = "Child — Today", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun ChildTodayPreview() { MastTheme { FamilyWardenScreen(screenState, previewActions) } }

@Preview(name = "Child — time is up, dark", uiMode = Configuration.UI_MODE_NIGHT_YES,
    showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun TimesUpPreview() { MastTheme {
    FamilyWardenScreen(screenState.copy(screenTime = screenTimeState(screenApps,
        screenReport.copy(used = mapOf(UsageCounter.TOTAL to 120 * 60_000L)), screenNow, 1000, true)), previewActions)
} }

@Preview(name = "Child — five minutes left", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun NearLimitPreview() { MastTheme {
    FamilyWardenScreen(screenState.copy(screenTime = screenTimeState(screenApps,
        screenReport.copy(used = mapOf(UsageCounter.TOTAL to 115 * 60_000L)), screenNow, 1000, true)), previewActions)
} }

@Preview(name = "Child — downtime", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun DowntimePreview() { MastTheme {
    val apps = screenApps.copy(downtime = listOf(Window(setOf(screenNow.dayOfWeek),
        java.time.LocalTime.of(18, 0), java.time.LocalTime.of(20, 0))))
    FamilyWardenScreen(screenState.copy(screenTime = screenTimeState(apps, screenReport, screenNow, 1000, true)), previewActions)
} }

@Preview(name = "Child — Danish at 200 percent", locale = "da", fontScale = 2f,
    showBackground = true, widthDp = 360, heightDp = 800)
@Composable
private fun ChildLargePreview() { MastTheme { FamilyWardenScreen(screenState, previewActions) } }

@Preview(name = "Child — bound by a pact, no asking", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun ChildPactPreview() { MastTheme {
    FamilyWardenScreen(screenState.copy(pact = PactStatus.Bound(true, null, Pact(null, null, null))), previewActions)
} }

@Preview(name = "Child — usage unavailable", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun ChildUnknownPreview() { MastTheme {
    FamilyWardenScreen(screenState.copy(appsStatus = "usage_access_missing",
        screenTime = screenTimeState(screenApps, screenReport, screenNow, 1000, false)), previewActions)
} }

// Adult (captain) fixtures: apps rest 21:30–06:30; the pact locks hotspot at the same instant.
private val adultApps = screenApps.copy(downtime = listOf(Window(java.time.DayOfWeek.entries.toSet(),
    java.time.LocalTime.of(21, 30), java.time.LocalTime.of(6, 30))))
private val adultLock = screenNow.withHour(21).withMinute(30)
private val adultPact = PactStatus.Bound(false, adultLock.toInstant().toEpochMilli(),
    Pact(screenNow.plusDays(3).withHour(18).toInstant().toEpochMilli(), null, null),
    adultLock.plusDays(1).withHour(6).withMinute(30).toInstant().toEpochMilli())
private val adultState = screenState.copy(binding = previewState.binding, pact = adultPact, timeZone = screenNow.zone.id,
    screenTime = screenTimeState(adultApps, screenReport, screenNow, 1000, true))
private fun adultTime(used: Long) = screenTimeState(adultApps,
    screenReport.copy(used = mapOf(UsageCounter.TOTAL to used, "com.example.video" to 26 * 60_000L)), screenNow, 1000, true)

@Preview(name = "Adult — open until, pact", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun AdultTodayPreview() { MastTheme { FamilyWardenScreen(adultState, previewActions) } }

@Preview(name = "Adult — five minutes left", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun AdultNearLimitPreview() { MastTheme {
    FamilyWardenScreen(adultState.copy(screenTime = adultTime(115 * 60_000L)), previewActions)
} }

@Preview(name = "Adult — time is up, dark", uiMode = Configuration.UI_MODE_NIGHT_YES,
    showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun AdultTimesUpPreview() { MastTheme {
    FamilyWardenScreen(adultState.copy(screenTime = adultTime(120 * 60_000L)), previewActions)
} }

@Preview(name = "Adult — resting, pact locked", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun AdultDowntimePreview() { MastTheme {
    val night = screenNow.withHour(22)
    val morning = night.plusDays(1).withHour(6).withMinute(30).toInstant().toEpochMilli()
    FamilyWardenScreen(adultState.copy(nowMillis = night.toInstant().toEpochMilli(),
        pact = adultPact.copy(blocked = true, nextChangeMillis = morning, followingChangeMillis = null),
        screenTime = screenTimeState(adultApps, screenReport, night, 1000, true)), previewActions)
} }

@Preview(name = "Adult — usage unavailable", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun AdultUnknownPreview() { MastTheme {
    FamilyWardenScreen(adultState.copy(appsStatus = "usage_gap", screenTime = screenTimeState(adultApps,
        screenReport.copy(reliable = false), screenNow, 1000, true)), previewActions)
} }

@Preview(name = "Adult — pact only, Danish at 200 percent", locale = "da", fontScale = 2f,
    showBackground = true, widthDp = 360, heightDp = 800)
@Composable
private fun AdultPactLargePreview() { MastTheme {
    FamilyWardenScreen(adultState.copy(screenTime = null, appsActive = false), previewActions)
} }

@Preview(name = "FamilyWarden — applied DNS", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun OverviewPreview() { MastTheme { FamilyWardenScreen(previewState, previewActions) } }

@Preview(name = "FamilyWarden — captain and cached policy", showBackground = true,
    uiMode = Configuration.UI_MODE_NIGHT_YES, widthDp = 400, heightDp = 840)
@Composable
private fun CaptainOfflinePreview() { MastTheme {
    FamilyWardenScreen(previewState.copy(syncError = "router unreachable",
        pact = PactStatus.Bound(true, null, Pact(null, null, null))), previewActions)
} }

@Preview(name = "FamilyWarden — unsupported router", showBackground = true, widthDp = 400, heightDp = 840)
@Composable
private fun UnsupportedPreview() { MastTheme {
    FamilyWardenScreen(FamilyWardenState(previewNow, owner = true, paired = true, policyUnsupported = true), previewActions)
} }

@Preview(name = "FamilyWarden — Danish, large text", locale = "da", fontScale = 2f,
    showBackground = true, widthDp = 360, heightDp = 800)
@Composable
private fun DanishLargePreview() { MastTheme {
    FamilyWardenScreen(previewState.copy(networkId = 2), previewActions)
} }
