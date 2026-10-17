package io.github.denogio.familywarden.ui

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.path
import androidx.compose.ui.unit.dp

internal object MastIcons {
    /** Brand mark: a mast with a single sail over a calm wave. Tinted by Icon. */
    val Mast: ImageVector by lazy {
        ImageVector.Builder(
            name = "MastMark",
            defaultWidth = 24.dp,
            defaultHeight = 24.dp,
            viewportWidth = 24f,
            viewportHeight = 24f,
        ).apply {
            path(
                stroke = SolidColor(Color.Black),
                strokeLineWidth = 1.8f,
                strokeLineCap = StrokeCap.Round,
                strokeLineJoin = StrokeJoin.Round,
            ) {
                // Mast
                moveTo(12f, 2.5f)
                verticalLineTo(19.5f)
                // Sail
                moveTo(12f, 4.5f)
                lineTo(18.5f, 14.5f)
                horizontalLineTo(12f)
                // Sea
                moveTo(4f, 19.5f)
                curveTo(6.7f, 21.1f, 9.3f, 21.1f, 12f, 19.5f)
                curveTo(14.7f, 17.9f, 17.3f, 17.9f, 20f, 19.5f)
            }
        }.build()
    }
}
