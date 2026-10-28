package io.github.denogio.familywarden

import org.json.JSONObject

/**
 * The router's signed household freshness: an epoch with a counter, plus the
 * fork points of earlier epochs left by restores. A phone keeps its last
 * trusted policy instead of accepting older household state, e.g. an older
 * backup restored onto a replacement router. Later router writes cannot catch
 * up: a fork point that is too low for this phone's epoch stays too low.
 */
data class HouseholdGeneration(val epoch: String, val counter: Long, val lineage: List<Pair<String, Long>>) {
    companion object {
        private val epochPattern = Regex("[0-9a-f]{32}")
        private const val MAX_COUNTER = 1L shl 53
        private const val MAX_LINEAGE = 16

        fun parse(o: JSONObject): HouseholdGeneration {
            val epoch = o.getString("epoch")
            val counter = o.getLong("counter")
            val raw = o.getJSONArray("lineage")
            require(epoch.matches(epochPattern) && counter in 0..MAX_COUNTER && raw.length() <= MAX_LINEAGE) { "invalid generation" }
            val lineage = (0 until raw.length()).map { i ->
                val a = raw.getJSONObject(i)
                (a.getString("epoch") to a.getLong("counter")).also { (e, c) ->
                    require(e.matches(epochPattern) && c in 0..MAX_COUNTER) { "invalid generation lineage" }
                }
            }
            require((lineage.map { it.first } + epoch).toSet().size == lineage.size + 1) { "repeated generation epoch" }
            return HouseholdGeneration(epoch, counter, lineage)
        }

        /** Whether [next] continues what this phone last accepted ([previous]). */
        fun follows(previous: HouseholdGeneration?, next: HouseholdGeneration?): Boolean = when {
            previous == null -> true
            next == null -> false // a router that stopped vouching is a downgrade
            next.epoch == previous.epoch -> next.counter >= previous.counter
            else -> next.lineage.any { (epoch, counter) -> epoch == previous.epoch && counter >= previous.counter }
        }
    }
}
