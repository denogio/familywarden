package io.github.denogio.familywarden

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.net.InetAddress
import java.net.ServerSocket
import java.time.LocalDate
import java.util.concurrent.atomic.AtomicReference
import kotlin.concurrent.thread

class FamilyHistoryClientTest {
    private val router = PhoneKey.generate()
    private val phone = PhoneKey.generate()
    private val profile = ProfileRef("alex", "Alex")
    private val day = LocalDate.parse("2027-01-15")
    private val now = 1800000000L
    private val raw = FamilyHistoryClient.query("fw-parent", profile.id, day, now, "a".repeat(64))
    private fun reply(request: String = raw) = JSONObject().put("v", 1).put("type", "history_reply").put("device", "fw-parent")
        .put("server_time", now).put("message_sha256", RequestProtocol.digest(request.toByteArray()))
        .put("profile_id", "alex").put("day", day.toString()).put("reported", true).put("total", 0)
        .put("devices", JSONArray().put(JSONObject().put("name", "Alex phone").put("total", 0)
            .put("incomplete", false).put("received", now - 300)))
        .put("apps", JSONArray())
    private fun signed(o: JSONObject): ByteArray = o.toString().toByteArray().let { (router.sign(it) + "\n").toByteArray() + it }
    private fun parse(bytes: ByteArray) = FamilyHistoryClient.parse(bytes, router.publicKey, "fw-parent", profile, day, raw, now)
    private fun refuses(block: () -> Unit) { try { block(); fail("unsafe history accepted") } catch (_: Exception) { } }

    @Test fun missingAndReportedZeroAreDifferentAndReceiptIsNotCurrentTime() {
        val zero = parse(signed(reply()))
        assertEquals(0, zero.usage!!.totalSeconds)
        assertEquals(now - 300, zero.usage.devices.single().received)
        val missing = reply().put("reported", false).put("devices", JSONArray())
        assertNull(parse(signed(missing)).usage)
        assertEquals(day.toString(), zero.day)
        // Receipt is additive: old sync replies still parse without fabricating one.
        reply().also { it.getJSONArray("devices").getJSONObject(0).remove("received") }.let {
            assertNull(parse(signed(it)).usage!!.devices.single().received)
        }
    }

    @Test fun exactQuerySignatureTimeProfileAndCoverageAreRequired() {
        val bad = listOf(
            reply().put("type", "request_reply"), reply().put("device", "fw-alex"), reply().put("server_time", now - 1),
            reply().put("message_sha256", "0".repeat(64)), reply().put("profile_id", "sam"), reply().put("day", "2027-01-14"),
            reply().put("reported", false), reply().put("reported", "true"),
            reply().put("devices", JSONArray()), reply().put("total", 0.5),
            reply().also { it.getJSONArray("devices").getJSONObject(0).put("received", "1800000000") }
        )
        bad.forEach { refuses { parse(signed(it)) } }
        val changed = raw.replace("a".repeat(64), "b".repeat(64))
        refuses { FamilyHistoryClient.parse(signed(reply()), router.publicKey, "fw-parent", profile, day, changed, now) }
        val altered = signed(reply()).also { it[it.lastIndex] = ' '.code.toByte() }
        refuses { parse(altered) }
        refuses { FamilyHistoryClient.parse(signed(reply()), phone.publicKey, "fw-parent", profile, day, raw, now) }
        refuses { parse(ByteArray(70000)) }
    }

    @Test fun signedRefusalsAndOldRoutersNeverBecomeEmptyHistory() {
        for (code in listOf("not_parent", "unavailable", "day_out_of_range", "request_expired")) {
            try { parse(signed(reply().put("error", code))); fail() }
            catch (e: HistoryRefused) { assertEquals(code, e.code) }
        }
        refuses { parse(signed(reply().put("error", "invented"))) }
        refuses { FamilyHistoryClient.query("fw-parent", "../sam", day, now, "a".repeat(64)) }
        refuses { FamilyHistoryClient.query("fw-parent", "alex", day, now, "old-nonce") }
        ServerSocket(0, 1, InetAddress.getByName("127.0.0.1")).let { listener ->
            val endpoint = "127.0.0.1:${listener.localPort}"
            listener.close()
            refuses { FamilyHistoryClient.read(endpoint, router.publicKey, phone, "fw-parent", profile, day, now) }
        }
    }

    @Test fun actualHttpReadUsesItsOwnSigningDomainAndFreshNonce() {
        val listener = ServerSocket(0, 1, InetAddress.getByName("127.0.0.1"))
        val failure = AtomicReference<Throwable?>()
        val worker = thread(isDaemon = true) {
            try { listener.accept().use { socket ->
                socket.soTimeout = 5000
                val input = socket.getInputStream()
                val headers = ByteArrayOutputStream()
                while (!headers.toString("US-ASCII").endsWith("\r\n\r\n")) {
                    val c = input.read(); check(c >= 0 && headers.size() < 8192); headers.write(c)
                }
                val lines = headers.toString("US-ASCII").split("\r\n")
                assertEquals("POST /cgi-bin/app?op=history HTTP/1.1", lines.first())
                val count = lines.first { it.startsWith("Content-Length:", true) }.substringAfter(':').trim().toInt()
                val body = ByteArray(count)
                var read = 0
                while (read < count) { val n = input.read(body, read, count - read); check(n > 0); read += n }
                val request = body.copyOfRange(101, count).toString(Charsets.UTF_8)
                val query = JSONObject(request)
                assertEquals("history", query.getString("type"))
                assertTrue(query.getString("nonce").matches(Regex("[0-9a-f]{64}")))
                assertTrue(Usign.verify(Usign.parsePublicKey(phone.publicKey), body.copyOfRange(0, 100).toString(Charsets.US_ASCII),
                    "familywarden-history-v1\n".toByteArray() + request.toByteArray()))
                val bytes = signed(reply(request))
                socket.getOutputStream().write("HTTP/1.1 200 OK\r\nContent-Length: ${bytes.size}\r\nConnection: close\r\n\r\n".toByteArray() + bytes)
            } } catch (e: Throwable) { failure.set(e) }
        }
        try {
            assertEquals(0, FamilyHistoryClient.read("127.0.0.1:${listener.localPort}", router.publicKey, phone,
                "fw-parent", profile, day, now).usage!!.totalSeconds)
        } finally { listener.close(); worker.join(6000); failure.get()?.let { throw it } }
    }
}
