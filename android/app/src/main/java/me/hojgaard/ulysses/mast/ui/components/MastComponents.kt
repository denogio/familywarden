package me.hojgaard.ulysses.mast.ui.components

import android.content.ClipData
import android.content.ClipboardManager
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.delay
import me.hojgaard.ulysses.mast.R
import me.hojgaard.ulysses.mast.ui.theme.MastSpacing
import me.hojgaard.ulysses.mast.ui.theme.MastThemeExt
import me.hojgaard.ulysses.mast.ui.theme.MonoMedium

// ---------------------------------------------------------------- Hero cards

/** The big status card on top of the screen. */
@Composable
fun HeroCard(
    modifier: Modifier = Modifier,
    color: Color = MaterialTheme.colorScheme.surfaceContainer,
    decoration: (@Composable () -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.extraLarge,
        color = color,
    ) {
        Box {
            decoration?.let { Box(Modifier.align(Alignment.TopEnd).padding(top = 18.dp, end = 18.dp)) { it() } }
            Column(Modifier.padding(MastSpacing.heroPadding), content = content)
        }
    }
}

/** Night-sea card, identical in light and dark: the "locked" state. */
@Composable
fun NightCard(
    modifier: Modifier = Modifier,
    showStars: Boolean = true,
    content: @Composable ColumnScope.() -> Unit,
) {
    val night = MastThemeExt.colors
    Box(
        modifier
            .fillMaxWidth()
            .clip(MaterialTheme.shapes.extraLarge)
            .background(
                Brush.linearGradient(
                    colors = listOf(night.nightTop, night.nightBottom),
                    start = Offset.Zero,
                    end = Offset.Infinite,
                ),
            ),
    ) {
        if (showStars) Stars(Modifier.align(Alignment.TopEnd).padding(top = 16.dp, end = 18.dp))
        Column(Modifier.padding(MastSpacing.heroPadding), content = content)
    }
}

/** A handful of stars. Decoration only, hidden from TalkBack. */
@Composable
private fun Stars(modifier: Modifier = Modifier) {
    val night = MastThemeExt.colors
    Canvas(modifier.size(width = 96.dp, height = 44.dp).clearAndSetSemantics { }) {
        val u = size.width / 96f
        fun star(x: Float, y: Float, r: Float, c: Color) = drawCircle(c, radius = r * u, center = Offset(x * u, y * u))
        star(8f, 30f, 1.2f, night.nightStar)
        star(30f, 10f, 1.6f, night.nightAccent)
        star(52f, 26f, 1.0f, night.nightStar)
        star(70f, 6f, 1.2f, night.onNight)
        star(88f, 22f, 1.4f, night.nightStar)
    }
}

/** A short calm wave for the free-sailing card. Decoration only. */
@Composable
fun Wave(color: Color, modifier: Modifier = Modifier) {
    Canvas(modifier.size(width = 80.dp, height = 20.dp).clearAndSetSemantics { }) {
        val path = androidx.compose.ui.graphics.Path().apply {
            val w = size.width
            val h = size.height
            moveTo(0f, h * 0.6f)
            var x = 0f
            val step = w / 4f
            var up = true
            while (x < w - 1f) {
                quadraticTo(x + step / 2f, if (up) 0f else h, x + step, h * 0.6f)
                x += step
                up = !up
            }
        }
        drawPath(
            path,
            color = color,
            style = androidx.compose.ui.graphics.drawscope.Stroke(width = 1.6.dp.toPx()),
        )
    }
}

@Composable
fun HeroBadge(icon: ImageVector, container: Color, content: Color) {
    Box(
        Modifier
            .size(48.dp)
            .background(container, CircleShape),
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription = null, tint = content)
    }
    Spacer(Modifier.height(MastSpacing.l))
}

/**
 * State headline. It is a heading and a polite live region, so TalkBack announces
 * "Hotspot is locked" when the state flips while the screen is open.
 */
@Composable
fun HeroTitle(text: String, color: Color = Color.Unspecified) {
    Text(
        text = text,
        style = MaterialTheme.typography.headlineMedium,
        color = color,
        modifier = Modifier.semantics {
            heading()
            liveRegion = LiveRegionMode.Polite
        },
    )
}

@Composable
fun HeroBody(text: String, color: Color = MaterialTheme.colorScheme.onSurfaceVariant) {
    Spacer(Modifier.height(MastSpacing.s))
    Text(text, style = MaterialTheme.typography.bodyMedium, color = color)
}

/** Divider plus an icon line at the bottom of a hero card ("Opens in 8 h 24 min"). */
@Composable
fun HeroFooter(icon: ImageVector, text: String, textColor: Color, iconColor: Color, dividerColor: Color) {
    Spacer(Modifier.height(18.dp))
    HorizontalDivider(color = dividerColor)
    Spacer(Modifier.height(14.dp))
    Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(icon, contentDescription = null, tint = iconColor, modifier = Modifier.size(20.dp))
        Spacer(Modifier.width(MastSpacing.s))
        Text(text, style = MaterialTheme.typography.titleSmall.copy(fontSize = MaterialTheme.typography.bodyMedium.fontSize), color = textColor)
    }
}

// ---------------------------------------------------------------- Cards and text

@Composable
fun SectionCard(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainer,
    ) {
        Column(Modifier.padding(horizontal = MastSpacing.cardPadding, vertical = 18.dp), content = content)
    }
}

@Composable
fun Overline(text: String, color: Color = MaterialTheme.colorScheme.secondary) {
    Text(
        text = text.uppercase(),
        style = MaterialTheme.typography.labelSmall,
        color = color,
        modifier = Modifier.padding(bottom = 6.dp).semantics { contentDescription = text },
    )
}

/** Small muted line with a leading icon (e.g. "Pairing and removal open again..."). */
@Composable
fun InfoNote(icon: ImageVector, text: String, modifier: Modifier = Modifier) {
    Row(modifier.padding(horizontal = 6.dp, vertical = 2.dp)) {
        Icon(
            icon,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 1.dp).size(18.dp),
        )
        Spacer(Modifier.width(10.dp))
        Text(text, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

/** Neutral or warning banner above the hero card. */
@Composable
fun Banner(
    icon: ImageVector,
    title: String,
    body: String,
    modifier: Modifier = Modifier,
    warning: Boolean = false,
    extra: String? = null,
) {
    val colors = MaterialTheme.colorScheme
    Surface(
        modifier = modifier
            .fillMaxWidth()
            .semantics(mergeDescendants = true) {
                liveRegion = if (warning) LiveRegionMode.Assertive else LiveRegionMode.Polite
            },
        shape = MaterialTheme.shapes.medium,
        color = if (warning) colors.errorContainer else colors.surfaceContainerHigh,
        border = if (warning) BorderStroke(1.dp, colors.error.copy(alpha = 0.35f)) else null,
    ) {
        Row(Modifier.padding(horizontal = MastSpacing.l, vertical = 14.dp)) {
            Icon(
                icon,
                contentDescription = null,
                tint = if (warning) colors.error else colors.onSurfaceVariant,
                modifier = Modifier.size(22.dp),
            )
            Spacer(Modifier.width(MastSpacing.m))
            Column(verticalArrangement = Arrangement.spacedBy(MastSpacing.xs)) {
                Text(
                    title,
                    style = if (warning) MaterialTheme.typography.titleSmall else MaterialTheme.typography.labelMedium,
                    color = if (warning) colors.error else colors.onSurface,
                )
                Text(body, style = MaterialTheme.typography.bodySmall, color = if (warning) colors.onSurface else colors.onSurfaceVariant)
                if (extra != null) {
                    Text(extra, style = MaterialTheme.typography.bodySmall, color = colors.onSurface)
                }
            }
        }
    }
}

// ---------------------------------------------------------------- Setup

@Composable
fun SetupStep(number: Int, text: String) {
    Row(verticalAlignment = Alignment.Top) {
        Box(
            Modifier
                .size(24.dp)
                .background(MaterialTheme.colorScheme.surfaceContainerHigh, CircleShape)
                .clearAndSetSemantics { },
            contentAlignment = Alignment.Center,
        ) {
            Text(number.toString(), style = MaterialTheme.typography.labelMedium.copy(fontSize = MaterialTheme.typography.labelSmall.fontSize))
        }
        Spacer(Modifier.width(MastSpacing.m))
        Text(
            text,
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.semantics { contentDescription = "$number. $text" },
        )
    }
}

/** Monospace command with a copy button. Copying is a UI concern, so it lives here. */
@Composable
fun CommandBlock(command: String, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    var copied by remember { mutableStateOf(false) }
    LaunchedEffect(copied) {
        if (copied) {
            delay(2_000)
            copied = false
        }
    }
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = MaterialTheme.shapes.small,
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
    ) {
        Row(Modifier.padding(start = 14.dp, top = 4.dp, bottom = 4.dp, end = 4.dp), verticalAlignment = Alignment.Top) {
            Text(
                command,
                style = MonoMedium.copy(fontSize = MaterialTheme.typography.bodySmall.fontSize),
                modifier = Modifier
                    .weight(1f)
                    .padding(vertical = 10.dp),
            )
            IconButton(onClick = {
                context.getSystemService(ClipboardManager::class.java)
                    ?.setPrimaryClip(ClipData.newPlainText("adb", command))
                copied = true
            }) {
                Icon(
                    if (copied) Icons.Outlined.Check else Icons.Outlined.ContentCopy,
                    contentDescription = stringResource(if (copied) R.string.copied else R.string.action_copy_command),
                )
            }
        }
    }
}

// ---------------------------------------------------------------- Details

data class DetailItem(
    val label: String,
    val value: String,
    val mono: Boolean = false,
    val spoken: String? = null,
)

/**
 * Compact key/value list. At large font scales the rows stack (label above value)
 * so nothing gets squeezed or truncated.
 */
@Composable
fun DetailsCard(items: List<DetailItem>, modifier: Modifier = Modifier) {
    val title = stringResource(R.string.details_title)
    Surface(
        modifier = modifier
            .fillMaxWidth()
            .semantics { contentDescription = title },
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.surfaceContainer,
    ) {
        Column(Modifier.padding(horizontal = MastSpacing.cardPadding, vertical = MastSpacing.xs)) {
            items.forEachIndexed { index, item ->
                if (index > 0) HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
                DetailRow(item)
            }
        }
    }
}

@Composable
private fun DetailRow(item: DetailItem) {
    val stacked = LocalDensity.current.fontScale >= 1.5f
    val labelStyle = MaterialTheme.typography.bodySmall
    val valueStyle: TextStyle = if (item.mono) MonoMedium else MaterialTheme.typography.bodySmall
    val rowModifier = Modifier
        .fillMaxWidth()
        .heightIn(min = 40.dp)
        .padding(vertical = 10.dp)
        .semantics(mergeDescendants = true) {
            item.spoken?.let { contentDescription = "${item.label}: $it" }
        }
    if (stacked) {
        Column(rowModifier, verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(item.label, style = labelStyle, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Text(item.value, style = valueStyle, color = MaterialTheme.colorScheme.onSurface)
        }
    } else {
        Row(rowModifier, verticalAlignment = Alignment.CenterVertically) {
            Text(item.label, style = labelStyle, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.width(MastSpacing.m))
            Text(
                item.value,
                style = valueStyle,
                color = MaterialTheme.colorScheme.onSurface,
                textAlign = TextAlign.End,
                modifier = Modifier.weight(1f),
            )
        }
    }
}
