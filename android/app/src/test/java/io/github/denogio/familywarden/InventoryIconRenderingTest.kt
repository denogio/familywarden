package io.github.denogio.familywarden

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.ColorFilter
import android.graphics.Paint
import android.graphics.PixelFormat
import android.graphics.drawable.AdaptiveIconDrawable
import android.graphics.drawable.ColorDrawable
import android.graphics.drawable.Drawable
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.util.Base64

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class InventoryIconRenderingTest {
    private fun render(drawable: Drawable): Bitmap {
        drawable.setBounds(1, 2, 31, 42)
        val before = android.graphics.Rect(drawable.bounds)
        val bytes = Base64.getDecoder().decode(InventoryUpload.icon(drawable)!!)
        assertTrue(bytes.size <= InventoryIconUpload.MAX_ICON_BYTES)
        assertEquals(before, drawable.bounds)
        return BitmapFactory.decodeByteArray(bytes, 0, bytes.size).also {
            assertEquals(96, it.width)
            assertEquals(96, it.height)
        }
    }

    @Test fun adaptiveLayersFillSquareWithoutLauncherCircleMask() {
        val foreground = object : Drawable() {
            override fun draw(canvas: Canvas) {
                canvas.drawCircle(bounds.exactCenterX(), bounds.exactCenterY(), bounds.width() / 4f, Paint().apply { color = Color.GREEN })
            }
            override fun setAlpha(alpha: Int) {}
            override fun setColorFilter(filter: ColorFilter?) {}
            @Deprecated("Deprecated in Android") override fun getOpacity() = PixelFormat.TRANSLUCENT
        }
        val image = render(AdaptiveIconDrawable(ColorDrawable(Color.RED), foreground))
        try {
            assertEquals(Color.RED, image.getPixel(0, 0))
            assertEquals(Color.RED, image.getPixel(95, 95))
            assertEquals(Color.GREEN, image.getPixel(48, 48))
        } finally { image.recycle() }
    }

    @Test fun adaptiveVisible72Of108RegionReachesViewportEdgesAndNullLayersAreAllowed() {
        val foreground = object : Drawable() {
            override fun draw(canvas: Canvas) {
                // The central 72/108 region is Android's visible viewport.
                val dx = bounds.width() / 6f
                val dy = bounds.height() / 6f
                canvas.drawRect(bounds.left + dx, bounds.top + dy, bounds.right - dx, bounds.bottom - dy,
                    Paint().apply { color = Color.GREEN })
            }
            override fun setAlpha(alpha: Int) {}
            override fun setColorFilter(filter: ColorFilter?) {}
            @Deprecated("Deprecated in Android") override fun getOpacity() = PixelFormat.TRANSLUCENT
        }
        val image = render(AdaptiveIconDrawable(null, foreground))
        try {
            for ((x, y) in listOf(0 to 48, 95 to 48, 48 to 0, 48 to 95)) {
                assertEquals("72/108 edge must reach viewport ($x,$y)", Color.GREEN, image.getPixel(x, y))
            }
        } finally { image.recycle() }
        val backgroundOnly = render(AdaptiveIconDrawable(ColorDrawable(Color.RED), null))
        try {
            assertEquals(Color.RED, backgroundOnly.getPixel(0, 0))
            assertEquals(Color.RED, backgroundOnly.getPixel(48, 48))
        } finally { backgroundOnly.recycle() }
    }

    @Test fun transparentWideLegacyArtworkRetainsAspectAndOpaqueNeutralSquare() {
        val drawable = object : Drawable() {
            override fun getIntrinsicWidth() = 80
            override fun getIntrinsicHeight() = 40
            override fun draw(canvas: Canvas) {
                canvas.drawRect(bounds.left.toFloat(), bounds.top.toFloat(), bounds.right.toFloat(), bounds.bottom.toFloat(), Paint().apply { color = Color.BLUE })
            }
            override fun setAlpha(alpha: Int) {}
            override fun setColorFilter(filter: ColorFilter?) {}
            @Deprecated("Deprecated in Android") override fun getOpacity() = PixelFormat.TRANSLUCENT
        }
        val image = render(drawable)
        try {
            val neutral = Color.rgb(245, 245, 245)
            assertEquals(neutral, image.getPixel(0, 0))
            assertEquals(neutral, image.getPixel(48, 23))
            assertEquals(Color.BLUE, image.getPixel(48, 24))
            assertEquals(Color.BLUE, image.getPixel(48, 71))
            assertEquals(neutral, image.getPixel(48, 72))
            for (x in 0 until 96) for (y in 0 until 96) assertEquals(255, Color.alpha(image.getPixel(x, y)))
        } finally { image.recycle() }
    }
}
