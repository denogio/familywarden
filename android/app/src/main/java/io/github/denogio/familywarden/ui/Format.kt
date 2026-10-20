package io.github.denogio.familywarden.ui

import android.text.format.DateFormat
import androidx.compose.runtime.Composable
import androidx.compose.runtime.State
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalInspectionMode
import androidx.compose.ui.res.stringResource
import kotlinx.coroutines.delay
import io.github.denogio.familywarden.R
import java.time.Instant
import java.time.ZoneId
import java.time.temporal.ChronoUnit
import java.util.Date

// Presentation-only formatting. Respects the system 12/24 h setting and locale.

/** "21:30" or "9:30 PM". */
@Composable
internal fun formatClock(millis: Long): String {
    val context = LocalContext.current
    return remember(millis, context) { DateFormat.getTimeFormat(context).format(Date(millis)) }
}

/** "Tue 3 Nov" (locale-aware skeleton). */
@Composable
internal fun formatDay(millis: Long): String {
    val locale = LocalConfiguration.current.locales[0]
    return remember(millis, locale) {
        val pattern = DateFormat.getBestDateTimePattern(locale, "EEEdMMM")
        DateFormat.format(pattern, millis).toString()
    }
}

/** "today at 21:30", "tomorrow at 06:00", "Tue 3 Nov at 06:00". Lower case, for sentences. */
@Composable
internal fun formatWhen(millis: Long, nowMillis: Long): String {
    val zone = ZoneId.systemDefault()
    val days = ChronoUnit.DAYS.between(
        Instant.ofEpochMilli(nowMillis).atZone(zone).toLocalDate(),
        Instant.ofEpochMilli(millis).atZone(zone).toLocalDate(),
    )
    val time = formatClock(millis)
    return when (days) {
        0L -> stringResource(R.string.when_today, time)
        1L -> stringResource(R.string.when_tomorrow, time)
        -1L -> stringResource(R.string.when_yesterday, time)
        else -> stringResource(R.string.when_date, formatDay(millis), time)
    }
}

/** Same as [formatWhen] with a capital first letter, for detail rows. */
@Composable
internal fun formatWhenCapitalized(millis: Long, nowMillis: Long): String =
    formatWhen(millis, nowMillis).replaceFirstChar { it.titlecase() }

/** "8 h 24 min", "12 min", "2 d 3 h". Rounds up so it never says "0 min" too early. */
@Composable
internal fun formatDuration(millis: Long): String {
    val totalMinutes = (millis.coerceAtLeast(0) + 59_999) / 60_000
    val days = totalMinutes / (60 * 24)
    val hours = (totalMinutes / 60) % 24
    val minutes = totalMinutes % 60
    return when {
        days > 0 -> stringResource(R.string.duration_days_hours, days.toInt(), hours.toInt())
        hours > 0 -> stringResource(R.string.duration_hours_minutes, hours.toInt(), minutes.toInt())
        else -> stringResource(R.string.duration_minutes, minutes.toInt())
    }
}

/** "4:12:08". */
internal fun formatCountdown(millis: Long): String {
    val total = millis.coerceAtLeast(0) / 1000
    val h = total / 3600
    val m = (total / 60) % 60
    val s = total % 60
    return "%d:%02d:%02d".format(h, m, s)
}

/** "e7e3 3ab8 f182 801f". */
internal fun groupFingerprint(fp: String): String = fp.chunked(4).joinToString(" ")

/** Read character by character by TalkBack: "e 7 e 3, 3 a b 8, ...". */
internal fun spokenFingerprint(fp: String): String =
    fp.chunked(4).joinToString(", ") { group -> group.toList().joinToString(" ") }

/**
 * A ticking clock for countdowns. Purely visual; when the countdown reaches zero the
 * Activity's own scheduling (or the next sync) moves the state on. Frozen in previews.
 */
@Composable
internal fun rememberNow(initialMillis: Long, periodMillis: Long): State<Long> {
    val inPreview = LocalInspectionMode.current
    val clock = remember { mutableStateOf(initialMillis) }
    LaunchedEffect(inPreview, periodMillis) {
        if (!inPreview) {
            while (true) {
                clock.value = System.currentTimeMillis()
                delay(periodMillis)
            }
        }
    }
    return clock
}
