package io.github.denogio.familywarden

import org.json.JSONObject
import java.time.LocalTime
import java.time.ZoneId

/**
 * v1 is readable for diagnostics only; v2 is explicitly scoped and request-bound;
 * v3 is v2 plus the signed app policy ("apps").
 */
object Payload {
    /** Epoch seconds through the year 9999: rejects milliseconds and overflowing alarms. */
    private const val MAX_EPOCH = 253402300799L

    fun parse(json: String): MastState {
        val o = JSONObject(json)
        val version = o.optInt("v")
        require(version in 1..3) { "unsupported payload version" }
        val scoped = version >= 2
        val nonce = if (scoped) o.getString("nonce").also {
            require(it.matches(Regex("[0-9a-f]{64}"))) { "invalid response nonce" }
        } else null
        val device = if (scoped) o.getString("device").also {
            require(it.matches(Regex("[A-Za-z0-9-]{1,80}"))) { "invalid device ID" }
        } else null
        PolicyCapabilities.check(o)
        if (scoped) when (o.getString("enrollment")) {
            // Signed "not approved (yet)": nothing about pacts, roles, DNS or apps
            "pending" -> return MastState(
                o.getLong("server_time").also { require(it in 0..MAX_EPOCH) { "invalid epoch" } },
                "", false, "", 0, 0, Rules(false, "allow", "none", emptyList()),
                requestNonce = nonce, wireVersion = version, device = device, pending = true,
            )
            "approved" -> Unit
            else -> throw IllegalArgumentException("unknown enrollment state")
        }
        val binding = if (scoped) o.getJSONObject("binding").let { b ->
            DeviceBinding(
                b.getString("device_id"), b.getString("profile_id"), b.getString("profile_name"),
                b.getBoolean("captain"), b.getBoolean("administrator"), b.getBoolean("can_self_remove"),
                if (version == 3) approves(b) else emptyList(),
            ).also {
                require(it.deviceId.matches(Regex("[A-Za-z0-9-]{1,80}"))) { "invalid device ID" }
                require(it.profileId.matches(Regex("[A-Za-z0-9_-]{1,64}"))) { "invalid profile ID" }
                require(it.profileName.length in 1..128) { "invalid profile name" }
                require(it.deviceId == device) { "binding for another device" }
                require(it.approves.none { ref -> ref.id == it.profileId }) { "self-approval scope" }
            }
        } else null
        val dnsObject = if (scoped && o.has("dns") && !o.isNull("dns")) o.getJSONObject("dns") else null
        val dns = dnsObject?.let { d ->
            val names = d.getJSONArray("protections")
            require(names.length() <= Protection.entries.size) { "too many DNS protections" }
            val hostname = d.getString("hostname")
            require(validHostname(hostname)) { "invalid DNS hostname" }
            val services = d.optJSONArray("services")
            require((services?.length() ?: 0) <= 500) { "too many blocked services" }
            DnsPolicy(hostname.lowercase(), (0 until names.length()).map {
                Protection.valueOf(names.getString(it).uppercase())
            }.toSet(), (0 until (services?.length() ?: 0)).map { i ->
                services!!.getString(i).also { require(it.matches(Regex("[a-z0-9_]{1,64}"))) { "invalid service" } }
            })
        }
        // Only v3 carries app policy; an "apps" field in an older version is not trusted.
        val apps = if (version == 3 && o.has("apps") && !o.isNull("apps")) parseApps(o.getJSONObject("apps")) else null
        // Optional: routers without a valid IANA zone name leave it out (phone zone is used)
        val zone = if (scoped) o.optString("timezone").takeIf { it.isNotEmpty() }?.also { ZoneId.of(it) } else null
        val pact = if (scoped) o.getJSONObject("pact") else o.optJSONObject("pact") ?: JSONObject()
        val r = if (scoped) o.getJSONObject("rules") else o.optJSONObject("rules") ?: JSONObject()
        val windows = if (scoped) r.getJSONArray("windows") else r.optJSONArray("windows")
        require((windows?.length() ?: 0) <= 128) { "too many schedule windows" }
        val state = MastState(
            serverTime = o.getLong("server_time"),
            captain = o.optString("captain"),
            pactActive = if (scoped) pact.getBoolean("active") else pact.optBoolean("active"),
            pactType = if (scoped) pact.getString("type") else pact.optString("type"),
            pactUntil = if (scoped) pact.getLong("until") else pact.optLong("until"),
            releaseAt = if (scoped) pact.getLong("release_at") else pact.optLong("release_at"),
            rules = Rules(
                enabled = r.optString("enabled") == "1",
                policy = r.optString("policy", "allow"),
                override = r.optString("override", "none"),
                windows = (0 until (windows?.length() ?: 0)).mapNotNull { i ->
                    val w = windows!!.getJSONObject(i)
                    if (scoped) window(w) else try {
                        Window(Policy.parseDays(w.getString("days")), LocalTime.parse(w.getString("start")),
                            LocalTime.parse(w.getString("stop")))
                    } catch (_: Exception) { null }
                },
            ),
            binding = binding, dns = dns, requestNonce = nonce, wireVersion = version, timeZone = zone,
            device = device, apps = apps,
            parents = if (version == 3 && binding != null) parents(o, binding.deviceId) else emptyList(),
        )
        // Bounded epoch seconds (through year 9999), not milliseconds/overflowing alarm times.
        require(listOf(state.serverTime, state.pactUntil, state.releaseAt).all {
            it in 0..MAX_EPOCH
        }) { "invalid epoch" }
        if (scoped) {
            require(r.getString("enabled") in setOf("0", "1")) { "invalid enabled flag" }
            require(r.getString("policy") in setOf("allow", "block", "scheduled")) { "invalid policy" }
            require(r.getString("override") in setOf("none", "allow", "block")) { "invalid override" }
            require(state.pactType in setOf("", "ongoing", "until")) { "invalid pact type" }
            if (binding?.captain == true) require(state.captain == binding.profileId) { "captain mismatch" }
            if (state.pactActive) {
                require(binding?.captain == true && state.captain == binding.profileId) { "pact is not applicable to this device" }
                require(state.pactType in setOf("ongoing", "until")) { "invalid pact type" }
                require(state.pactType != "until" || state.pactUntil > 0) { "invalid pact end" }
                // No grants beyond the pact (A4/A17): a bound captain trusts no parent keys.
                require(state.parents.isEmpty()) { "parent keys for a pact-bound phone" }
            }
        }
        return state
    }

    private val profileId = Regex("[A-Za-z0-9_-]{1,64}")

    /** Optional; strict when present. Unique profiles, never the phone's own (checked by caller). */
    private fun approves(b: JSONObject): List<ProfileRef> {
        val arr = b.optJSONArray("approves") ?: return emptyList()
        require(arr.length() <= 16) { "too many approved profiles" }
        return (0 until arr.length()).map { i ->
            val o = arr.getJSONObject(i)
            ProfileRef(o.getString("profile_id").also { require(it.matches(profileId)) { "invalid profile ID" } },
                o.getString("profile_name").also { require(it.length in 1..128) { "invalid profile name" } })
        }.also { refs -> require(refs.map { it.id }.toSet().size == refs.size) { "duplicate approved profile" } }
    }

    /** Keys are parsed now so a malformed key can never be trusted later for grants. */
    private fun parents(o: JSONObject, self: String): List<ParentKey> {
        val arr = o.optJSONArray("parents") ?: return emptyList()
        require(arr.length() <= 16) { "too many parents" }
        return (0 until arr.length()).map { i ->
            val p = arr.getJSONObject(i)
            ParentKey(p.getString("device_id").also { require(it.matches(Regex("[A-Za-z0-9-]{1,80}"))) { "invalid device ID" } },
                p.getString("name").also { require(it.length in 1..128) { "invalid parent name" } },
                p.getString("pubkey").also { Usign.parsePublicKey(it) })
        }.also { keys ->
            require(keys.map { it.deviceId }.toSet().size == keys.size) { "duplicate parent" }
            require(keys.none { it.deviceId == self }) { "phone listed as its own parent" }
        }
    }

    /** Strict schedule window: known weekdays, "HH:MM" bounds. */
    private fun window(w: JSONObject): Window {
        val days = w.getString("days")
        val start = w.getString("start")
        val stop = w.getString("stop")
        require(days.isNotBlank() && days.trim().split(Regex("\\s+")).all {
            Policy.parseDays(it).size == 1
        }) { "invalid schedule days" }
        require(start.matches(Regex("[0-9]{2}:[0-9]{2}")) &&
            stop.matches(Regex("[0-9]{2}:[0-9]{2}"))) { "invalid schedule time" }
        return Window(Policy.parseDays(days), LocalTime.parse(start), LocalTime.parse(stop))
    }

    /**
     * Rejects rather than repairs: a contradictory or oversized policy keeps the
     * last trusted one in force (the reply is refused as unreadable).
     */
    private fun parseApps(a: JSONObject): AppsPolicy {
        fun packages(name: String, max: Int): List<String> {
            val arr = a.getJSONArray(name)
            require(arr.length() <= max) { "too many $name" }
            return (0 until arr.length()).map { i ->
                arr.getString(i).also { require(validPackage(it)) { "invalid package" } }
            }.also { require(it.toSet().size == it.size) { "duplicate package in $name" } }
        }
        val rev = a.getString("rev")
        require(rev.matches(Regex("[0-9a-f]{16}"))) { "invalid apps revision" }
        val limitArray = a.getJSONArray("limits")
        require(limitArray.length() <= 256) { "too many limits" }
        val limits = (0 until limitArray.length()).associate { i ->
            val l = limitArray.getJSONObject(i)
            val pkg = l.getString("package").also { require(validPackage(it)) { "invalid package" } }
            pkg to l.getInt("minutes").also { require(it in 0..1440) { "invalid minutes" } }
        }
        require(limits.size == limitArray.length()) { "duplicate limit" }
        val blocked = packages("blocked", 256).toSet()
        val approved = packages("approved", 512).toSet()
        require(blocked.intersect(approved).isEmpty()) { "package both blocked and approved" }
        require(approved.containsAll(limits.keys)) { "limit for an unapproved package" }
        val down = a.getJSONArray("downtime")
        require(down.length() <= 32) { "too many downtime windows" }
        // D9 fields are optional: a router without them sends none.
        val screen = a.optJSONObject("screen")?.let { s ->
            require(s.length() <= 7) { "too many screen days" }
            s.keys().asSequence().associate { key ->
                val day = Policy.parseDays(key).singleOrNull()
                require(day != null && key.length == 3) { "invalid screen day" }
                day to s.getInt(key).also { require(it in 0..1440) { "invalid screen minutes" } }
            }
        } ?: emptyMap()
        val always = if (a.has("always_allowed")) packages("always_allowed", 256).toSet() else emptySet()
        val bonus = a.optJSONObject("bonus")
        val bonusDay = bonus?.getString("day")?.let { java.time.LocalDate.parse(it) }
        val bonusApps = bonus?.optJSONArray("apps")?.let { arr ->
            require(arr.length() <= 64) { "too many app bonuses" }
            (0 until arr.length()).associate { i ->
                val b = arr.getJSONObject(i)
                b.getString("package").also { require(validPackage(it)) { "invalid package" } } to
                    b.getInt("minutes").also { require(it in 1..1440) { "invalid bonus minutes" } }
            }.also { require(it.size == arr.length()) { "duplicate app bonus" } }
        } ?: emptyMap()
        val bonusTotal = bonus?.optInt("total", 0) ?: 0
        require(bonusTotal in 0..1440) { "invalid bonus minutes" }
        return AppsPolicy(rev, a.getBoolean("suspend_new"), limits, blocked, approved,
            (0 until down.length()).map { window(down.getJSONObject(it)) },
            screen, always, bonusDay, bonusTotal, bonusApps)
    }
}
