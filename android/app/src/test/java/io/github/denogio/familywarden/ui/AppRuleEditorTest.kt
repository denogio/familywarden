package io.github.denogio.familywarden.ui

import io.github.denogio.familywarden.R
import org.junit.Assert.assertEquals
import org.junit.Test

class AppRuleEditorTest {
    @Test fun helpTextTracksTheSelectedDraftMode() {
        assertEquals(R.string.fw_apps_rule_allowed_body, ruleDraftBody("allowed"))
        assertEquals(R.string.fw_apps_rule_limit_body, ruleDraftBody("limit"))
        assertEquals(R.string.fw_apps_rule_always_body, ruleDraftBody("always"))
        assertEquals(R.string.fw_apps_rule_blocked_body, ruleDraftBody("blocked"))
    }
}
