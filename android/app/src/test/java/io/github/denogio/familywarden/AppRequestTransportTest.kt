package io.github.denogio.familywarden

import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.net.InetAddress
import java.net.ServerSocket
import java.util.concurrent.atomic.AtomicReference
import kotlin.concurrent.thread

/** Actual bounded HTTP POST and usign bytes, without Android networking mocks. */
class AppRequestTransportTest {
    private val router = PhoneKey.generate()
    private val phone = PhoneKey.generate()
    private val now = 1800000000L
    private fun upload() = RequestUpload("request", RequestProtocol.request("transport-001", "fw-alex", "alex",
        "com.example.video", 30, now), router.publicKey, "fw-alex")

    private fun server(status: Int = 200, oversized: Boolean = false, action: RequestUpload = upload(), test: (String) -> Unit) {
        val listener = ServerSocket(0, 4, InetAddress.getByName("127.0.0.1"))
        val failure = AtomicReference<Throwable?>()
        val worker = thread(isDaemon = true) {
            try { listener.accept().use { socket ->
                socket.soTimeout = 5000
                val input = socket.getInputStream()
                val headers = ByteArrayOutputStream()
                while (!headers.toString("US-ASCII").endsWith("\r\n\r\n")) {
                    check(headers.size() < 8192)
                    val byte = input.read()
                    check(byte >= 0)
                    headers.write(byte)
                }
                val lines = headers.toString("US-ASCII").split("\r\n")
                assertEquals("POST /cgi-bin/app?op=${action.op} HTTP/1.1", lines.first())
                val size = lines.first { it.startsWith("Content-Length:", true) }.substringAfter(':').trim().toInt()
                check(size <= if (action.op == "inventory_icons") 64 * 1024 else 16 * 1024)
                val body = ByteArray(size)
                var at = 0
                while (at < size) { val read = input.read(body, at, size - at); check(read > 0); at += read }
                assertArrayEquals(action.signedBody(phone), body)
                val signature = body.copyOfRange(0, 100).toString(Charsets.US_ASCII)
                val raw = body.copyOfRange(101, body.size)
                assertTrue(Usign.verify(Usign.parsePublicKey(phone.publicKey), signature,
                    "familywarden-${action.op}-v1\n".toByteArray() + raw))
                val reply = JSONObject().put("v", 1).put("type", "${action.op}_reply").put("device", "fw-alex")
                    .put("server_time", now).put("message_sha256", RequestProtocol.digest(raw))
                    .put("request_id", "transport-001").put("state", if (action.op == "inventory_icons") "stored" else "pending").toString().toByteArray()
                val bytes = if (oversized) ByteArray(70_000) else (router.sign(reply) + "\n").toByteArray() + reply
                socket.getOutputStream().use { out ->
                    out.write("HTTP/1.1 $status Test\r\nContent-Length: ${bytes.size}\r\nConnection: close\r\nLocation: http://127.0.0.1:1/rogue\r\n\r\n".toByteArray())
                    out.write(bytes)
                }
            } } catch (e: Throwable) { if (!listener.isClosed) failure.set(e) }
        }
        try { test("127.0.0.1:${listener.localPort}") } finally {
            listener.close()
            worker.join(6000)
            failure.get()?.let { throw it }
        }
    }

    @Test fun postPreservesExactMessageAndVerifiesRouterReply() {
        server { endpoint -> assertEquals(UploadReply("pending", null), AppRequestClient.post(endpoint, upload(), phone, now)) }
    }

    @Test fun independentIconUploadUsesExactSignedLargeEnvelope() {
        val image = java.util.Base64.getEncoder().encodeToString(ByteArray(InventoryIconUpload.MAX_ICON_BYTES))
        val items = listOf(InventoryUpload.Item("com.example.video", "Video", false, false, image))
        val raw = InventoryIconUpload.batches("fw-alex", "alex", 1, 1, items).single()
        assertTrue(raw.toByteArray().size > 16 * 1024)
        val action = RequestUpload("inventory_icons", raw, router.publicKey, "fw-alex")
        server(action = action) { endpoint -> assertEquals(UploadReply("stored", null), AppRequestClient.post(endpoint, action, phone, now)) }
    }

    @Test fun redirectsErrorsAndOversizedRepliesNeverBecomeAuthority() {
        for ((status, size) in listOf(307 to false, 503 to false, 200 to true)) server(status, size) { endpoint ->
            try { AppRequestClient.post(endpoint, upload(), phone, now); fail("unsafe HTTP reply accepted") }
            catch (_: Exception) { }
        }
    }

    @Test fun parentOnlyPolicyCannotClaimToEnforceManagedAppsDnsOrPacts() {
        val parent = MastState(now, "", false, "", 0, 0, Rules(false, "allow", "none", emptyList()),
            binding = DeviceBinding("fw-parent", "parent", "Taylor", false, false, false, listOf(ProfileRef("alex", "Alex"))), wireVersion = 3)
        assertTrue(parent.parentOnlyCompatible())
        assertFalse(parent.copy(wireVersion = 2).parentOnlyCompatible())
        assertFalse(parent.copy(pactActive = true).parentOnlyCompatible())
        assertFalse(parent.copy(binding = parent.binding!!.copy(captain = true)).parentOnlyCompatible())
        assertFalse(parent.copy(dns = DnsPolicy("alex.dns.example.net", emptySet())).parentOnlyCompatible())
        assertFalse(parent.copy(apps = AppsPolicy("0123456789abcdef", false, emptyMap(), emptySet(), emptySet(), emptyList())).parentOnlyCompatible())
    }
}
