package io.github.denogio.familywarden

import org.json.JSONArray
import org.json.JSONObject
import java.time.Instant
import java.time.LocalDateTime
import java.time.ZoneId
import java.time.ZonedDateTime

/** Same atomic checkpoint as UsageCounter: spent minutes and the event cursor never diverge. */
class GrantLedger {
    data class Spend(val scope: String, val grant: AppGrant, var millis: Long = 0, var uncertainMillis: Long = 0)
    data class Context(val scope: String, val device: String, val appsJson: String, val zone: ZoneId,
        val wall: Long, val server: Long, val excluded: Set<String>, val userApps: Set<String>, val baselineReliable: Boolean,
        val uncertainThroughDay: String = "",
        /** Other phones' usage on [sharedDay] (D4): added, never subtracted. */
        val sharedDay: String = "", val shared: Map<String, Long> = emptyMap()) {
        val apps: AppsPolicy = Payload.parseApps(JSONObject(appsJson), device)
    }
    private val records = linkedMapOf<String, Spend>()
    private val floors = linkedMapOf<String, Long>()
    private var context: Context? = null
    private var gapFrom: Long? = null

    private fun key(scope: String, id: String) = "$scope/$id"
    private fun end(g: AppGrant) = g.notAfter * 1000
    private fun start(g: AppGrant) = g.notBefore * 1000
    private fun budget(g: AppGrant) = g.minutes * 60_000L
    private fun remaining(s: Spend) = (budget(s.grant) - s.millis).coerceAtLeast(0)

    /** Called after consuming the old interval, before persisting and enforcing the new policy. */
    fun configure(next: Context): Boolean {
        require(next.scope.length <= 320 && next.scope.isNotBlank())
        require(next.wall in 0..253402300799000L && next.server in 1..253402300799000L)
        require(next.excluded.size <= 512 && next.userApps.size <= 4096)
        if (next.server < (floors[next.scope] ?: 0)) return false
        for (g in next.apps.grants) {
            val old = records[key(next.scope, g.id)]
            require(old == null || old.grant == g) { "grant terms changed" }
        }
        // A missing interval is an upper bound, never invented zero usage. Only the
        // possible overlap with each grant is charged, and it is visible separately.
        val old = context
        val missing = gapFrom
        if (old != null && missing != null) {
            records.values.filter { it.scope == old.scope }.forEach { s ->
                val lost = if (next.scope != old.scope) remaining(s) else
                    (minOf(next.server, end(s.grant)) - maxOf(missing, start(s.grant))).coerceAtLeast(0)
                        .coerceAtMost(remaining(s))
                s.millis += lost
                s.uncertainMillis += lost
            }
        }
        advance(next.scope, next.server)
        require(floors.size <= 32) { "too many grant trust contexts" }
        val newGrants = next.apps.grants.filter { end(it) > next.server && key(next.scope, it.id) !in records }
        require(records.size + newGrants.size <= 512) { "grant ledger full" }
        for (g in newGrants) records[key(next.scope, g.id)] = Spend(next.scope, g)
        context = next
        gapFrom = null
        return true
    }

    /** An untrusted clock/usage interval cannot authorize any extra time. */
    fun gap() {
        val c = context ?: return
        if (gapFrom == null) gapFrom = floors[c.scope] ?: c.server
    }

    /** Which target budgets can be used now; no local wall-clock expiry authority. */
    fun available(scope: String, serverMillis: Long?): Map<String, Long> {
        val c = context ?: return emptyMap()
        if (gapFrom != null || c.scope != scope || serverMillis == null || serverMillis < (floors[scope] ?: 0)) return emptyMap()
        return c.apps.grants.filter { serverMillis >= start(it) && serverMillis < end(it) }
            .groupBy(AppGrant::target).mapValues { (_, grants) ->
                grants.sumOf { records[key(scope, it.id)]?.let(::remaining) ?: 0 }
            }
    }

    fun previousExclusions(fallback: Set<String>): Set<String> = context?.excluded ?: fallback

    /** Spent milliseconds of this phone's grants still relevant at [sinceServerMillis] (usage reports). */
    fun reported(scope: String, device: String, sinceServerMillis: Long): Map<String, Long> =
        records.values.filter { it.scope == scope && it.grant.deviceId == device && end(it.grant) > sinceServerMillis }
            .associate { it.grant.id to it.millis }

    fun spent(scope: String, id: String): Long = records[key(scope, id)]?.millis ?: 0
    fun uncertain(scope: String, id: String): Long = records[key(scope, id)]?.uncertainMillis ?: 0

    /** UsageCounter calls before adding a continuous, screen-on/unlocked daily slice.
     * App grants spend only when ordinary rules would deny use. PiP total use is a
     * union; two visible apps cannot spend a total grant twice at the same instant.
     */
    fun charge(wallStart: Long, wallStop: Long, visible: Set<String>, localBefore: Map<String, Long>) {
        val c = context ?: return
        if (gapFrom != null || wallStop <= wallStart) return
        // UsageCounter splits slices at midnight, so one call is one router-local day.
        val sliceDay = Instant.ofEpochMilli(wallStart).atZone(c.zone).toLocalDate().toString()
        val usedBefore = if (sliceDay != c.sharedDay) localBefore
            else localBefore + c.shared.mapValues { (k, v) -> (localBefore[k] ?: 0) + v }
        val serverStart = c.server + (wallStart - c.wall)
        require(serverStart >= (floors[c.scope] ?: 0)) { "grant clock moved backwards" }
        val eligible = visible - c.excluded
        val countsTotal = eligible.isNotEmpty()
        var at = wallStart
        while (at < wallStop) {
            val instant = Instant.ofEpochMilli(at)
            val now = instant.atZone(c.zone)
            val policy = c.apps.toLocal(c.zone, now.toLocalDate())
            val reliable = c.baselineReliable || (c.uncertainThroughDay.isNotEmpty() && now.toLocalDate().toString() > c.uncertainThroughDay)
            val offset = at - wallStart
            val total = (usedBefore[UsageCounter.TOTAL] ?: 0) + if (countsTotal) offset else 0
            val server = c.server + (at - c.wall)
            var stop = wallStop
            fun cut(candidate: Long) { if (candidate > at) stop = minOf(stop, candidate) }
            fun budgetCut(left: Long) { if (left > 0) cut(at + left) }
            if (countsTotal) policy.dailyLimitMillis?.let { budgetCut(it - total) }
            for (pkg in visible) policy.limitsMillis[pkg]?.let { budgetCut(it - (usedBefore[pkg] ?: 0) - offset) }
            // Civil-time boundaries, including both occurrences of a folded DST time.
            for (w in policy.downtime) for (time in listOf(w.start, w.stop)) {
                val local = LocalDateTime.of(now.toLocalDate(), time)
                val offsets = c.zone.rules.getValidOffsets(local)
                if (offsets.isEmpty()) cut(local.atZone(c.zone).toInstant().toEpochMilli())
                else for (zoneOffset in offsets) cut(ZonedDateTime.ofLocal(local, c.zone, zoneOffset).toInstant().toEpochMilli())
            }
            c.zone.rules.nextTransition(instant)?.instant?.toEpochMilli()?.let(::cut)
            cut(now.toLocalDate().plusDays(1).atStartOfDay(c.zone).toInstant().toEpochMilli())
            val active = c.apps.grants.mapNotNull { g -> records[key(c.scope, g.id)] }.filter { s ->
                cut(c.wall + start(s.grant) - c.server)
                cut(c.wall + end(s.grant) - c.server)
                server >= start(s.grant) && server < end(s.grant) && remaining(s) > 0
            }.sortedWith(compareBy({ it.grant.notAfter }, { it.grant.id }))
            val selected = linkedSetOf<Spend>()
            for (pkg in eligible.sorted()) {
                val used = (usedBefore[pkg] ?: 0) + offset
                val action = appAction(policy, pkg, false, pkg in c.userApps, used, total, now, reliable)
                if (action in setOf(AppAction.ALLOW, AppAction.BLOCKED, AppAction.NOT_APPROVED)) continue
                val specific = active.firstOrNull { it.grant.target == pkg }
                val appLimit = policy.limitsMillis[pkg]
                val appCap = appLimit != null && (used >= appLimit || !reliable)
                val chosen = specific ?: if (!appCap) active.firstOrNull { it.grant.target == "total" } else null
                if (chosen != null) selected.add(chosen)
            }
            selected.forEach { budgetCut(remaining(it)) }
            val delta = stop - at
            check(delta > 0)
            selected.forEach { it.millis = Math.addExact(it.millis, delta) }
            at = stop
        }
    }

    /** Clock high-water comes from elapsed time, not usage-event wall timestamps.
     * Even a millisecond of wall/elapsed capture jitter must not reject every later sample.
     */
    fun finishInterval(elapsedMillis: Long) {
        require(elapsedMillis >= 0)
        val c = context ?: return
        if (gapFrom == null) advance(c.scope, Math.addExact(floors.getValue(c.scope), elapsedMillis))
    }

    private fun advance(scope: String, server: Long) {
        require(server >= (floors[scope] ?: 0))
        floors[scope] = server
        // Preserve the time floor even after tombstones are pruned: rolling back
        // time cannot turn a pruned, expired id into a fresh allocation.
        records.entries.removeAll { (_, s) -> s.scope == scope && server >= end(s.grant) + 86_400_000 }
    }

    fun encode(): JSONObject = JSONObject().apply {
        put("version", 1)
        put("floors", JSONObject(floors as Map<*, *>))
        put("records", JSONArray(records.values.map { s -> JSONObject().apply {
            put("scope", s.scope); put("id", s.grant.id); put("device", s.grant.deviceId); put("target", s.grant.target)
            put("minutes", s.grant.minutes); put("from", s.grant.notBefore); put("until", s.grant.notAfter)
            put("used", s.millis); put("uncertain", s.uncertainMillis)
        } }))
        context?.let { c -> put("context", JSONObject().apply {
            put("scope", c.scope); put("device", c.device); put("apps", c.appsJson); put("zone", c.zone.id)
            put("wall", c.wall); put("server", c.server); put("excluded", JSONArray(c.excluded.toList()))
            put("users", JSONArray(c.userApps.toList())); put("reliable", c.baselineReliable)
            put("uncertain_day", c.uncertainThroughDay)
            put("shared_day", c.sharedDay); put("shared", JSONObject(c.shared as Map<*, *>))
        }) }
        gapFrom?.let { put("gap", it) }
    }

    companion object {
        fun decode(o: JSONObject): GrantLedger = GrantLedger().apply {
            require(RequestProtocol.integer(o, "version") == 1L)
            val fs = o.getJSONObject("floors")
            require(fs.length() <= 32)
            for (scope in fs.keys()) {
                require(scope.length in 1..320)
                floors[scope] = RequestProtocol.integer(fs, scope).also { require(it in 1..253402300799000L) }
            }
            val rs = o.getJSONArray("records")
            require(rs.length() <= 512)
            for (i in 0 until rs.length()) {
                val r = rs.getJSONObject(i)
                val scope = r.getString("scope").also { require(it in floors) }
                val g = AppGrant(r.getString("id"), r.getString("device"), r.getString("target"),
                    Math.toIntExact(RequestProtocol.integer(r, "minutes")), RequestProtocol.integer(r, "from"), RequestProtocol.integer(r, "until"))
                val used = RequestProtocol.integer(r, "used").also { require(it in 0..budget(g)) }
                val uncertain = RequestProtocol.integer(r, "uncertain").also { require(it in 0..used) }
                require(key(scope, g.id) !in records)
                records[key(scope, g.id)] = Spend(scope, g, used, uncertain)
            }
            if (o.has("context")) {
                val c = o.getJSONObject("context")
                fun packages(name: String, max: Int): Set<String> {
                    val a = c.getJSONArray(name)
                    require(a.length() <= max)
                    return (0 until a.length()).map { a.getString(it).also { pkg ->
                        require(pkg.length in 1..255 && pkg.matches(Regex("[A-Za-z][A-Za-z0-9_]*(\\.[A-Za-z0-9_]+)*")))
                    } }.toSet().also { require(it.size == a.length()) }
                }
                val ctx = Context(c.getString("scope"), c.getString("device"), c.getString("apps"), ZoneId.of(c.getString("zone")),
                    RequestProtocol.integer(c, "wall"), RequestProtocol.integer(c, "server"),
                    packages("excluded", 512), packages("users", 4096), c.getBoolean("reliable"),
                    c.optString("uncertain_day", "").also { require(it.isEmpty() || java.time.LocalDate.parse(it).toString() == it) },
                    c.optString("shared_day", "").also { require(it.isEmpty() || java.time.LocalDate.parse(it).toString() == it) },
                    c.optJSONObject("shared")?.let { o ->
                        require(o.length() <= 65)
                        o.keys().asSequence().associateWith { k ->
                            require(k == UsageCounter.TOTAL || validPackage(k))
                            RequestProtocol.integer(o, k).also { require(it in 0..16L * 25 * 3_600_000) }
                        }
                    } ?: emptyMap())
                require(ctx.scope in floors && ctx.wall in 0..253402300799000L && ctx.server in 1..floors.getValue(ctx.scope))
                for (g in ctx.apps.grants) require(records[key(ctx.scope, g.id)]?.grant == g || end(g) <= floors.getValue(ctx.scope))
                context = ctx
            }
            if (o.has("gap")) gapFrom = RequestProtocol.integer(o, "gap").also {
                val c = requireNotNull(context)
                require(it in 1..floors.getValue(c.scope))
            }
        }
    }
}
