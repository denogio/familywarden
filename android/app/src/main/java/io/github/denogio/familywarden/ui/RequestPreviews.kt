package io.github.denogio.familywarden.ui

import android.content.res.Configuration
import androidx.compose.runtime.Composable
import androidx.compose.ui.tooling.preview.Preview
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.ui.theme.MastTheme
import java.time.Instant
import java.time.ZoneId

private val actions = FamilyWardenActions({}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {})
private const val epoch = 1800000000L
private val alex = ProfileRef("alex", "Alex")
private val sam = ProfileRef("sam", "Sam")
private val parent = FamilyWardenState(epoch * 1000, owner = false, parentMode = true, paired = true, provisioned = true,
    host = "192.0.2.1", binding = DeviceBinding("fw-parent", "parent", "Taylor", false, false, false, listOf(alex, sam)),
    requestEpoch = epoch, parentRequests = listOf(
        ParentRequest("request-alex-001", "a".repeat(64), alex, "com.example.video", 30, epoch - 300, epoch + 3300, "Video"),
        ParentRequest("request-sam-001", "b".repeat(64), sam, "com.example.game", 15, epoch - 120, epoch + 3480, "Game")),
    familyUsage = listOf(FamilyUsage(alex, "2027-01-15", 82 * 60, listOf(DeviceUsage("Alex's phone", 70 * 60, false),
        DeviceUsage("Alex's tablet", 12 * 60, false)), listOf(AppSeconds("com.example.video", "Video", 30 * 60),
        AppSeconds("com.example.game", "Game", 25 * 60)))))
private val zone = ZoneId.of("Europe/Copenhagen")
private val now = Instant.ofEpochSecond(epoch).atZone(zone)
private val child = FamilyWardenState(epoch * 1000, owner = true, paired = true, provisioned = true,
    binding = DeviceBinding("fw-alex", "alex", "Alex", false, false, false), appsActive = true, appsStatus = "ok",
    requestEpoch = epoch, requestTargets = listOf(RequestTarget("com.example.video", "Video"), RequestTarget("com.example.game", "Game")),
    childRequest = ChildRequest("request-alex-001", "com.example.video", 30, "pending", epoch + 3300),
    screenTime = ScreenTimeState(TodayKind.TimesUp, 120 * 60_000L, 120 * 60_000L, null,
        now.toLocalDate().plusDays(1).atStartOfDay(zone), emptyList(), listOf("School"), emptyMap()))

@Preview(name = "Parent — requests", widthDp = 400, heightDp = 840)
@Composable
private fun ParentRequestsPreview() { MastTheme { FamilyWardenScreen(parent, actions) } }

@Preview(name = "Parent — requests, dark", uiMode = Configuration.UI_MODE_NIGHT_YES, widthDp = 400, heightDp = 840)
@Composable
private fun ParentRequestsDarkPreview() { MastTheme { FamilyWardenScreen(parent, actions) } }

@Preview(name = "Parent — Danish 200 percent", locale = "da", fontScale = 2f, widthDp = 360, heightDp = 800)
@Composable
private fun ParentRequestsLargePreview() { MastTheme { FamilyWardenScreen(parent, actions) } }

@Preview(name = "Child — request pending", widthDp = 400, heightDp = 840)
@Composable
private fun ChildRequestPreview() { MastTheme { FamilyWardenScreen(child, actions, initialRequests = true) } }

@Preview(name = "Child — retry is not another request", widthDp = 400, heightDp = 840)
@Composable
private fun ChildRetryPreview() { MastTheme { FamilyWardenScreen(child.copy(uploadPending = true), actions, initialRequests = true) } }

@Preview(name = "Child — approved extra time", widthDp = 400, heightDp = 840)
@Composable
private fun ChildExtraPreview() { MastTheme { FamilyWardenScreen(child.copy(
    childRequest = child.childRequest!!.copy(state = "approved", granted = 20, notAfter = epoch + 7200, answeredBy = "Taylor"),
    screenTime = child.screenTime!!.copy(grantRemaining = mapOf("com.example.video" to 20 * 60_000L))), actions) } }

@Preview(name = "Child — extra time, dark", uiMode = Configuration.UI_MODE_NIGHT_YES, widthDp = 400, heightDp = 840)
@Composable
private fun ChildExtraDarkPreview() { MastTheme { FamilyWardenScreen(child.copy(
    screenTime = child.screenTime!!.copy(grantRemaining = mapOf("com.example.video" to 20 * 60_000L))), actions) } }
