package io.github.denogio.familywarden.ui

import android.content.res.Configuration
import androidx.compose.runtime.Composable
import androidx.compose.ui.tooling.preview.Preview
import io.github.denogio.familywarden.*
import io.github.denogio.familywarden.ui.theme.MastTheme

private val previewActions = FamilyWardenActions({}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {})
private val previewDns = DnsPolicy("alex.dns.example.net", setOf(Protection.FILTER_LISTS, Protection.SAFE_SEARCH))
private val previewNow = 1_800_000_000_000L
private val previewState = FamilyWardenState(
    nowMillis = previewNow, owner = true, paired = true, provisioned = true,
    binding = DeviceBinding("fw-example", "captain", "Alex", true, true, true),
    host = "192.0.2.1", lastSync = previewNow / 1000, desiredDns = previewDns,
    appliedDnsHost = previewDns.hostname, dnsLocked = true, networkId = 1,
    resolverCheck = ResolverCheck(previewDns, 1, previewNow, ProbeResult.ANSWERED),
)

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
