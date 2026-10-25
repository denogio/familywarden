package io.github.denogio.familywarden

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build

/** LAN sync notifications only. This is not an off-home push transport or approval authority. */
object RequestNotifications {
    fun update(context: Context, state: MastState) {
        val nm = context.getSystemService(NotificationManager::class.java)
        val prefs = context.getSharedPreferences("request-notifications", Context.MODE_PRIVATE)
        val old = prefs.getStringSet("pending", emptySet()).orEmpty()
        val current = state.requests.map { it.id }.toSet()
        for (id in old - current) nm.cancel("family-requests", id.hashCode())
        if (Build.VERSION.SDK_INT >= 33 && context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) return
        nm.createNotificationChannel(NotificationChannel("family-requests", context.getString(R.string.fw_requests), NotificationManager.IMPORTANCE_DEFAULT))
        val open = PendingIntent.getActivity(context, 77, Intent(context, MainActivity::class.java).putExtra("requests", true),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        for (r in state.requests.filter { it.id !in old }) {
            nm.notify("family-requests", r.id.hashCode(), Notification.Builder(context, "family-requests")
                .setSmallIcon(R.drawable.ic_lighthouse_mono)
                .setContentTitle(context.getString(R.string.fw_request_notification, r.profile.name))
                .setContentText(context.getString(R.string.fw_request_phone_check))
                .setVisibility(Notification.VISIBILITY_PRIVATE).setOnlyAlertOnce(true)
                .setContentIntent(open).setAutoCancel(true).build())
        }
        prefs.edit().putStringSet("pending", current).apply()
    }
}
