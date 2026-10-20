package io.github.denogio.familywarden.ui.theme

import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/**
 * Brand palette. Echoes the router's "sail page": a night sea (#1B2A3A -> #070B10),
 * mist text (#D8E1EA) and a gold accent (#C9A227).
 */
internal object MastPalette {
    val Night = Color(0xFF1B2A3A)
    val NightDeep = Color(0xFF070B10)
    val NightInk = Color(0xFF0A1018)
    val NightSurface = Color(0xFF121C27)
    val NightLine = Color(0xFF2A3B4D)
    val NightTonal = Color(0xFF22344A)

    val Mist = Color(0xFFD8E1EA)
    val MistMuted = Color(0xFFB4C4D3)
    val Slate = Color(0xFF8AA0B4)

    val Gold = Color(0xFFC9A227)
    val GoldBright = Color(0xFFE0BD4A)
    val GoldDeep = Color(0xFF7A5D00)
    val GoldInk = Color(0xFF1A1405)

    val Ink = Color(0xFF0F1B27)
    val InkMuted = Color(0xFF4A5D70)
    val Foam = Color(0xFFEEF2F6)
    val FoamHigh = Color(0xFFE2E8EE)
    val FoamLine = Color(0xFFCDD6DF)
    val FoamTonal = Color(0xFFDBE3EC)
    val White = Color(0xFFFFFFFF)

    val SeaGreen = Color(0xFF1F6A64)
    val SeaGreenTint = Color(0xFFDCEFEC)
    val SeaGreenLight = Color(0xFF7CC7BD)
    val SeaGreenDeep = Color(0xFF13302E)

    val Rust = Color(0xFF8F2F0C)
    val RustTint = Color(0xFFFBE7DC)
    val Coral = Color(0xFFFFB38A)
    val RustDeep = Color(0xFF3A2116)
}

internal val MastLightColors = with(MastPalette) {
    lightColorScheme(
        primary = Night,
        onPrimary = White,
        primaryContainer = FoamTonal,
        onPrimaryContainer = Night,
        secondary = GoldDeep,
        onSecondary = White,
        secondaryContainer = Color(0xFFF3E7BF),
        onSecondaryContainer = Color(0xFF3A2C00),
        tertiary = SeaGreen,
        onTertiary = White,
        tertiaryContainer = SeaGreenTint,
        onTertiaryContainer = Color(0xFF0B2E2B),
        error = Rust,
        onError = White,
        errorContainer = RustTint,
        onErrorContainer = Color(0xFF3B1204),
        background = Foam,
        onBackground = Ink,
        surface = Foam,
        onSurface = Ink,
        surfaceVariant = FoamHigh,
        onSurfaceVariant = InkMuted,
        surfaceContainerLowest = White,
        surfaceContainerLow = White,
        surfaceContainer = White,
        surfaceContainerHigh = FoamHigh,
        surfaceContainerHighest = FoamLine,
        outline = Color(0xFF8193A5),
        outlineVariant = FoamLine,
        scrim = NightDeep,
        inverseSurface = Night,
        inverseOnSurface = Mist,
        inversePrimary = Gold,
    )
}

internal val MastDarkColors = with(MastPalette) {
    darkColorScheme(
        primary = Gold,
        onPrimary = GoldInk,
        primaryContainer = NightTonal,
        onPrimaryContainer = Mist,
        secondary = Gold,
        onSecondary = GoldInk,
        secondaryContainer = Color(0xFF3A3010),
        onSecondaryContainer = Color(0xFFF3E7BF),
        tertiary = SeaGreenLight,
        onTertiary = Color(0xFF00201D),
        tertiaryContainer = SeaGreenDeep,
        onTertiaryContainer = Mist,
        error = Coral,
        onError = Color(0xFF3B1204),
        errorContainer = RustDeep,
        onErrorContainer = Color(0xFFFFDBCB),
        background = NightInk,
        onBackground = Mist,
        surface = NightInk,
        onSurface = Mist,
        surfaceVariant = Night,
        onSurfaceVariant = Slate,
        surfaceContainerLowest = NightDeep,
        surfaceContainerLow = Color(0xFF0E1620),
        surfaceContainer = NightSurface,
        surfaceContainerHigh = Night,
        surfaceContainerHighest = NightTonal,
        outline = Color(0xFF5B6F83),
        outlineVariant = NightLine,
        scrim = Color(0xFF000000),
        inverseSurface = Mist,
        inverseOnSurface = Night,
        inversePrimary = GoldDeep,
    )
}

/**
 * Colours that Material roles do not cover. The "night" card is the same in both
 * themes: when the hotspot is locked, night falls, whatever the system theme.
 */
@Immutable
data class MastExtendedColors(
    val nightTop: Color,
    val nightBottom: Color,
    val onNight: Color,
    val onNightMuted: Color,
    val nightAccent: Color,
    val nightDivider: Color,
    val nightStar: Color,
)

internal val MastNightColors = with(MastPalette) {
    MastExtendedColors(
        nightTop = Night,
        nightBottom = NightDeep,
        onNight = Mist,
        onNightMuted = MistMuted,
        nightAccent = GoldBright,
        nightDivider = Mist.copy(alpha = 0.16f),
        nightStar = Slate,
    )
}

val LocalMastColors = staticCompositionLocalOf { MastNightColors }
