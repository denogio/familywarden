package io.github.denogio.familywarden

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject

/** One current phone of the profile; [reported] false means no snapshot, not zero apps. */
data class AppSource(val name: String, val reported: Boolean, val received: Long?, val installed: Int?, val sent: Int?) {
    val partial: Boolean get() = reported && (installed ?: 0) > (sent ?: 0)
}

/** The standing rule the router compiles today; display only, never local authority. */
sealed interface AppRule {
    data object None : AppRule
    data object Allowed : AppRule
    data object Always : AppRule
    data object Blocked : AppRule
    data class Limit(val minutes: Int) : AppRule
}

data class InstalledApp(val pkg: String, val label: String, val on: Set<Int>, val system: Set<Int>,
    val essential: Set<Int>, val rule: AppRule)

data class InstalledApps(val profile: ProfileRef, val sources: List<AppSource>, val apps: List<InstalledApp>,
    val appsRev: String, val suspendNew: Boolean, val serverTime: Long)

class InstalledRefused(val code: String) : IllegalStateException("installed apps refused: $code")

/** Read-only, like [FamilyHistoryClient]: no outbox, cache, acknowledgement or mutation. */
object FamilyAppsClient {
    private const val MAX_PAGES = 16

    fun apps(context: Context, profileId: String): InstalledApps = synchronized(Mast) {
        val store = Store(context)
        val fresh = Sync.fetch(store)
        check(fresh is Sync.Result.Ok && fresh.state.appliesTo(store.deviceId)) { "verified router required" }
        val binding = requireNotNull(fresh.state.binding)
        val profile = if (binding.profileId == profileId) ProfileRef(profileId, binding.profileName)
            else requireNotNull(binding.approves.singleOrNull { it.id == profileId }) { "profile outside current scope" }
        read(requireNotNull(store.publicKey), store.deviceId, profile, fresh.state.serverTime) { after, nonce ->
            val raw = query(store.deviceId, profile.id, after, fresh.state.serverTime, nonce)
            val signature = store.phoneKey.sign("familywarden-installed-v1\n".toByteArray() + raw.toByteArray(Charsets.UTF_8))
            raw to AppRequestClient.exchange(requireNotNull(store.host), "installed", (signature + "\n" + raw).toByteArray(Charsets.UTF_8))
        }
    }

    internal fun query(device: String, profile: String, after: String, created: Long, nonce: String): String {
        require(device.matches(Regex("[A-Za-z0-9-]{1,80}")) && profile.matches(Regex("[A-Za-z0-9_-]{1,64}")))
        require(created in 1..253402300799L && nonce.matches(Regex("[0-9a-f]{64}")) && (after.isEmpty() || validPackage(after)))
        return JSONObject().put("type", "installed").put("v", 1).put("device", device).put("profile", profile)
            .put("after", after).put("created", created).put("nonce", nonce).toString()
    }

    /**
     * Follows pages until done. A different snapshot id between pages means a phone
     * reported meanwhile: start over once rather than mixing two lists.
     */
    internal fun read(routerKey: String, device: String, profile: ProfileRef, serverTime: Long, exchange: (after: String, nonce: String) -> Pair<String, ByteArray>): InstalledApps {
        repeat(2) {
            var after = ""
            var first: InstalledApps? = null
            var snapshot: String? = null
            val apps = mutableListOf<InstalledApp>()
            for (page in 0 until MAX_PAGES) {
                val (raw, body) = exchange(after, Sync.newNonce())
                val (part, id, more) = parse(body, routerKey, device, profile, after, raw, serverTime)
                if (snapshot != null && id != snapshot) break
                snapshot = id
                if (first == null) first = part
                else require(part.sources == first.sources && part.appsRev == first.appsRev) { "inconsistent pages" }
                apps += part.apps
                if (!more) return first.copy(apps = apps)
                after = part.apps.last().pkg
            }
        }
        throw IllegalStateException("installed apps changed while reading")
    }

    internal fun parse(body: ByteArray, routerKey: String, device: String, profile: ProfileRef, after: String,
        raw: String, minimumServerTime: Long): Triple<InstalledApps, String, Boolean> {
        val o = RequestProtocol.signedReply(body, routerKey, device, "installed", raw, minimumServerTime)
        if (o.has("error")) {
            val code = o.get("error") as? String
            require(code in setOf("malformed", "not_parent", "request_expired", "app_control_off", "unavailable"))
            throw InstalledRefused(requireNotNull(code))
        }
        require(o.getString("profile_id") == profile.id && o.getString("after") == after)
        val snapshot = o.getString("snapshot").also { require(it.matches(Regex("[0-9a-f]{32}"))) }
        val rev = o.getString("apps_rev").also { require(it.length in 1..128) }
        val more = o.get("more") as? Boolean ?: throw IllegalArgumentException("invalid more")
        val suspendNew = o.get("suspend_new") as? Boolean ?: throw IllegalArgumentException("invalid suspend_new")
        val sources = o.getJSONArray("sources").let { a ->
            require(a.length() <= 32)
            (0 until a.length()).map { i ->
                val s = a.getJSONObject(i)
                val name = s.getString("name").also { require(it.codePointCount(0, it.length) in 1..64) }
                val reported = s.get("reported") as? Boolean ?: throw IllegalArgumentException("invalid source")
                if (!reported) AppSource(name, false, null, null, null)
                else {
                    val sent = RequestProtocol.integer(s, "sent").also { require(it in 0..512) }.toInt()
                    AppSource(name, true, RequestProtocol.integer(s, "received").also { require(it in 1..253402300799L) },
                        RequestProtocol.integer(s, "installed").also { require(it in sent..4096) }.toInt(), sent)
                }
            }
        }
        var previous = after
        val apps = o.getJSONArray("apps").let { a ->
            require(a.length() <= 2048 && (a.length() > 0 || !more))
            (0 until a.length()).map { i ->
                val x = a.getJSONObject(i)
                val pkg = x.getString("package").also { require(validPackage(it) && it > previous) }
                previous = pkg
                fun indices(key: String) = x.getJSONArray(key).let { arr ->
                    (0 until arr.length()).map { arr.getInt(it).also { n -> require(n in sources.indices && sources[n].reported) } }
                        .toSet().also { require(it.size == arr.length()) }
                }
                val on = indices("on")
                val system = indices("system").also { require(on.containsAll(it)) }
                val essential = indices("essential").also { require(on.containsAll(it)) }
                val rule = when (x.getString("rule")) {
                    "none" -> AppRule.None
                    "allowed" -> AppRule.Allowed
                    "always" -> AppRule.Always
                    "blocked" -> AppRule.Blocked
                    "limit" -> AppRule.Limit(RequestProtocol.integer(x, "minutes").also { require(it in 0..1440) }.toInt())
                    else -> throw IllegalArgumentException("unknown rule")
                }
                require(on.isNotEmpty() || rule != AppRule.None) { "app neither reported nor configured" }
                InstalledApp(pkg, UsageUpload.label(x.getString("label")) ?: "", on, system, essential, rule)
            }
        }
        return Triple(InstalledApps(profile, sources, apps, rev, suspendNew, RequestProtocol.integer(o, "server_time")), snapshot, more)
    }
}
