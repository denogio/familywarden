package io.github.denogio.familywarden.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp

val MastShapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(12.dp),   // code block, chips
    medium = RoundedCornerShape(20.dp),  // banners
    large = RoundedCornerShape(24.dp),   // cards
    extraLarge = RoundedCornerShape(28.dp), // hero card, dialogs, sheet
)

/** 4 dp grid. Screen gutter 16, cards 20-24 inside, 12 between sections. */
object MastSpacing {
    val xs = 4.dp
    val s = 8.dp
    val m = 12.dp
    val l = 16.dp
    val xl = 24.dp
    val xxl = 32.dp
    val gutter = 16.dp
    val cardPadding = 20.dp
    val heroPadding = 24.dp
    val buttonHeight = 52.dp
}

/**
 * @param dynamicColor Off by default: the brand is part of the calm, and the night card
 * keeps its own colours regardless. Turn on if you prefer wallpaper colours.
 */
@Composable
fun MastTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    dynamicColor: Boolean = false,
    content: @Composable () -> Unit,
) {
    val colorScheme = when {
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> {
            val context = LocalContext.current
            if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        }
        darkTheme -> MastDarkColors
        else -> MastLightColors
    }
    CompositionLocalProvider(LocalMastColors provides MastNightColors) {
        MaterialTheme(
            colorScheme = colorScheme,
            typography = MastTypography,
            shapes = MastShapes,
            content = content,
        )
    }
}

object MastThemeExt {
    val colors: MastExtendedColors
        @Composable get() = LocalMastColors.current
}
