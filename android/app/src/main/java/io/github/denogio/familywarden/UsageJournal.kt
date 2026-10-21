package io.github.denogio.familywarden

import android.content.Context
import android.util.AtomicFile
import org.json.JSONArray
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.File
import java.time.LocalDate
import java.time.ZoneId

/** One atomic checkpoint contains both cursor and cumulative totals: replay after a crash
 * starts at the last persisted cursor, never mixes new totals with an old cursor.
 */
data class UsageCheckpoint(
    val counter: UsageCounter,
    val zone: String,
    var elapsed: Long,
    var boot: Int,
    var sequence: Long = 0,
    var gaps: Long = 0,
    var uncertainThroughDay: String = "",
) {
    fun encode(): String = JSONObject().apply {
        put("schema", 1); put("zone", zone); put("cursor", counter.cursor)
        put("elapsed", elapsed); put("boot", boot); put("seq", sequence); put("gaps", gaps); put("uncertain_through_day", uncertainThroughDay)
        put("interactive", counter.interactive); put("unlocked", counter.unlocked)
        put("visible", JSONObject().apply { counter.visible.forEach { (pkg, ids) -> put(pkg, JSONArray(ids.toList())) } })
        put("days", JSONObject().apply { counter.days.forEach { (day, apps) -> put(day, JSONObject(apps as Map<*, *>)) } })
    }.toString()

    companion object {
        private fun packageName(value: String): Boolean = value.matches(Regex("[A-Za-z][A-Za-z0-9_]*(\\.[A-Za-z0-9_]+)*")) && value.length <= 255

        fun decode(raw: String): UsageCheckpoint {
            require(raw.toByteArray().size <= 1_048_576) { "usage checkpoint too large" }
            val o = JSONObject(raw)
            require(o.getInt("schema") == 1) { "unsupported usage schema" }
            val zone = o.getString("zone").also { ZoneId.of(it) }
            val cursor = o.getLong("cursor").also { require(it in 0..253402300799000L) }
            val c = UsageCounter(cursor, o.getBoolean("interactive"), o.getBoolean("unlocked"))
            val visible = o.getJSONObject("visible")
            require(visible.length() <= 512)
            for (pkg in visible.keys()) {
                require(packageName(pkg))
                val ids = visible.getJSONArray(pkg)
                require(ids.length() <= 64)
                c.visible[pkg] = (0 until ids.length()).map { ids.getString(it).also { id -> require(id.length <= 512) } }.toMutableSet()
            }
            val days = o.getJSONObject("days")
            require(days.length() <= 30)
            for (day in days.keys()) {
                require(LocalDate.parse(day).toString() == day)
                val apps = days.getJSONObject(day)
                require(apps.length() <= 512)
                c.days[day] = mutableMapOf<String, Long>().apply {
                    for (pkg in apps.keys()) {
                        require(packageName(pkg))
                        put(pkg, apps.getLong(pkg).also { require(it in 0..172_800_000L) })
                    }
                }
            }
            return UsageCheckpoint(c, zone, o.getLong("elapsed").also { require(it >= 0) },
                o.getInt("boot").also { require(it >= 0) },
                o.getLong("seq").also { require(it >= 0) }, o.getLong("gaps").also { require(it >= 0) },
                o.getString("uncertain_through_day").also { require(it.isEmpty() || LocalDate.parse(it).toString() == it) })
        }
    }
}

class UsageJournal(context: Context) {
    private val file = AtomicFile(File(context.filesDir, "app-usage.json"))

    /** Missing is first run. Corruption is an error, never an empty day's budget. */
    fun load(): UsageCheckpoint? {
        val input = try { file.openRead() } catch (e: java.io.FileNotFoundException) {
            if (file.baseFile.exists() || File(file.baseFile.path + ".bak").exists()) throw e
            return null
        }
        return input.use {
            val out = ByteArrayOutputStream()
            val buffer = ByteArray(4096)
            while (true) {
                val size = it.read(buffer)
                if (size < 0) break
                require(out.size() + size <= 1_048_576) { "usage checkpoint too large" }
                out.write(buffer, 0, size)
            }
            UsageCheckpoint.decode(out.toString("UTF-8"))
        }
    }

    fun save(state: UsageCheckpoint) {
        val bytes = state.encode().toByteArray(Charsets.UTF_8)
        // Validate bounds before replacing the previous good checkpoint.
        UsageCheckpoint.decode(String(bytes, Charsets.UTF_8))
        val output = file.startWrite()
        try {
            output.write(bytes)
            file.finishWrite(output)
        } catch (e: Exception) {
            file.failWrite(output)
            throw e
        }
    }
}
