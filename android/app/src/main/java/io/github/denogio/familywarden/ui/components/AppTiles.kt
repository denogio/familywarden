package io.github.denogio.familywarden.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

// Muted tile colours that keep white initials readable in both themes.
private val tileColors = listOf(
    Color(0xFF9A3B2E), Color(0xFF3F6B3A), Color(0xFF4B5568), Color(0xFF3D5A99),
    Color(0xFF8C6A2A), Color(0xFF2E6E5A), Color(0xFF6B4A8A), Color(0xFF7A4E3A),
)

/** Stable, distinguishable initials: one letter, two where first letters collide. */
fun tileInitials(labels: List<String>): Map<String, String> {
    val first = labels.groupBy { it.trim().take(1).uppercase() }
    return labels.associateWith { label ->
        val clean = label.trim()
        if (clean.isEmpty()) "?"
        else if ((first[clean.take(1).uppercase()]?.size ?: 0) > 1 && clean.length > 1)
            clean.take(1).uppercase() + clean[1].lowercaseChar()
        else clean.take(1).uppercase()
    }
}

/** Example-style app tile; decorative, so the label beside it carries the meaning. */
@Composable
fun AppTile(key: String, initials: String, size: Dp = 36.dp) {
    Box(
        Modifier.size(size).background(tileColors[Math.floorMod(key.hashCode(), tileColors.size)], RoundedCornerShape(10.dp))
            .clearAndSetSemantics { },
        contentAlignment = Alignment.Center,
    ) {
        Text(initials, color = Color.White, style = MaterialTheme.typography.labelLarge, fontWeight = FontWeight.SemiBold)
    }
}

/** A tile grid with captions ("Always OK"); [description] is read instead of each caption. */
@Composable
@OptIn(ExperimentalLayoutApi::class)
fun AppTileGrid(items: List<Pair<String, String>>, descriptions: Map<String, String> = emptyMap()) {
    val initials = tileInitials(items.map { it.second })
    FlowRow(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        items.forEach { (key, label) ->
            Column(
                // Labels wrap rather than truncate at 200 % font.
                Modifier.widthIn(min = 64.dp, max = 112.dp).semantics(mergeDescendants = true) { contentDescription = descriptions[key] ?: label },
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                AppTile(key, initials.getValue(label), 40.dp)
                Text(label, style = MaterialTheme.typography.labelSmall, textAlign = TextAlign.Center)
            }
        }
    }
}
