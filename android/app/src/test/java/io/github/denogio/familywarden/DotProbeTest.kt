package io.github.denogio.familywarden

import io.github.denogio.familywarden.ui.ProbeResult
import org.junit.Assert.*
import org.junit.Test

class DotProbeTest {
    private fun answer() = DotProbe.query(0xabcd).also { it[2] = 0x81.toByte(); it[3] = 0x80.toByte() }

    @Test fun encodesDnsQuestionAndAcceptsAnswerWithHighTransactionId() {
        val q = DotProbe.query(0xabcd)
        assertEquals(29, q.size)
        assertEquals(7, q[12].toInt())
        assertEquals("example", String(q.copyOfRange(13, 20)))
        assertEquals(ProbeResult.ANSWERED, DotProbe.check(answer(), 0xabcd))
    }

    @Test fun negativeDnsAnswerStillProvesResolverPathNotFiltering() {
        val response = answer().also { it[3] = 0x83.toByte() }
        assertEquals(ProbeResult.ANSWERED, DotProbe.check(response, 0xabcd))
    }

    @Test fun rejectsWrongIdTruncationOpcodeQuestionsAndDnsFailure() {
        assertEquals(ProbeResult.DNS_ERROR, DotProbe.check(answer(), 7))
        assertEquals(ProbeResult.DNS_ERROR, DotProbe.check(ByteArray(12), 0xabcd))
        for ((index, value) in listOf(2 to 0x83, 2 to 0x89, 3 to 0x85, 5 to 0, 13 to 0)) {
            val response = answer().also { it[index] = value.toByte() }
            assertEquals("byte $index = $value", ProbeResult.DNS_ERROR, DotProbe.check(response, 0xabcd))
        }
    }
}
