package io.github.denogio.familywarden

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class HouseholdGenerationTest {
    private val a = "a".repeat(32)
    private val b = "b".repeat(32)
    private val c = "c".repeat(32)
    private fun g(epoch: String, counter: Long, vararg lineage: Pair<String, Long>) =
        HouseholdGeneration(epoch, counter, lineage.toList())

    @Test fun sameEpochNeverGoesBackwards() {
        assertTrue(HouseholdGeneration.follows(g(a, 5), g(a, 5)))
        assertTrue(HouseholdGeneration.follows(g(a, 5), g(a, 9)))
        assertFalse(HouseholdGeneration.follows(g(a, 5), g(a, 4)))
    }

    @Test fun restoreIsAcceptedOnlyWhenItsForkPointCoversThisPhone() {
        // Newest backup restored: the fork point equals what the phone saw.
        assertTrue(HouseholdGeneration.follows(g(a, 5), g(b, 0, a to 5)))
        // Older backup: refused, and later router writes cannot catch up.
        assertFalse(HouseholdGeneration.follows(g(a, 5), g(b, 0, a to 4)))
        assertFalse(HouseholdGeneration.follows(g(a, 5), g(b, 1_000_000, a to 4)))
        // Repeated restores keep earlier fork points.
        assertTrue(HouseholdGeneration.follows(g(a, 5), g(c, 3, b to 2, a to 5)))
        // Unknown ancestry (e.g. a pre-generation backup) is not newer.
        assertFalse(HouseholdGeneration.follows(g(a, 5), g(b, 7)))
    }

    @Test fun onlyFirstContactOrNoHistoryAcceptsMissingGenerations() {
        assertTrue(HouseholdGeneration.follows(null, null))
        assertTrue(HouseholdGeneration.follows(null, g(a, 1)))
        assertFalse(HouseholdGeneration.follows(g(a, 1), null))
    }

    @Test fun parserRejectsMalformedOrAmbiguousGenerations() {
        val ok = """{"epoch":"$a","counter":2,"lineage":[{"epoch":"$b","counter":1}]}"""
        assertEquals(g(a, 2, b to 1), HouseholdGeneration.parse(JSONObject(ok)))
        for (bad in listOf(
            """{"epoch":"A${a.drop(1)}","counter":1,"lineage":[]}""",
            """{"epoch":"$a","counter":-1,"lineage":[]}""",
            """{"epoch":"$a","counter":1,"lineage":[{"epoch":"$a","counter":0}]}""",
            """{"epoch":"$a","counter":1,"lineage":[{"epoch":"$b","counter":0},{"epoch":"$b","counter":1}]}""",
            """{"epoch":"$a","counter":1}""",
        )) {
            assertThrows(Exception::class.java) { HouseholdGeneration.parse(JSONObject(bad)) }
        }
        val tooLong = (0..16).joinToString(",") { """{"epoch":"${"%032x".format(it + 1)}","counter":0}""" }
        assertThrows(Exception::class.java) {
            HouseholdGeneration.parse(JSONObject("""{"epoch":"$a","counter":1,"lineage":[$tooLong]}"""))
        }
    }
}
