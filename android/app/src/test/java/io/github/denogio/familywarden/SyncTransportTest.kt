package io.github.denogio.familywarden

import org.junit.Assert.*
import org.junit.Test
import java.io.OutputStream
import java.net.InetAddress
import java.net.ServerSocket
import java.net.URI
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicReference
import kotlin.concurrent.thread

/** Real local HTTP transport; no Android network mock, external DNS, or test dependency. */
class SyncTransportTest {
    private fun text(name: String) = javaClass.classLoader!!.getResourceAsStream(name)!!.readBytes().toString(Charsets.UTF_8)
    private val key = text("router.pub").trim()
    private val phone = PhoneKey.generate()
    private val signed = text("payload.sig").trim() + "\n" + text("payload.json")

    private fun reply(out: OutputStream, body: String, status: Int = 200, headers: String = "") {
        val bytes = body.toByteArray(Charsets.UTF_8)
        out.write("HTTP/1.1 $status Test\r\nContent-Length: ${bytes.size}\r\nConnection: close\r\n$headers\r\n".toByteArray(Charsets.US_ASCII))
        out.write(bytes)
    }

    // Android's unit-test boot classpath lacks jdk.httpserver. A tiny GET-only
    // socket fixture avoids adding a server library to test four transport cases.
    private fun server(handler: (URI, OutputStream) -> Unit, test: (String) -> Unit) {
        val server = ServerSocket(0, 8, InetAddress.getByName("127.0.0.1"))
        val failure = AtomicReference<Throwable?>()
        val worker = thread(isDaemon = true) {
            try {
                while (!server.isClosed) server.accept().use { socket ->
                    socket.soTimeout = 2000
                    val reader = socket.getInputStream().bufferedReader(Charsets.US_ASCII)
                    val uri = URI(reader.readLine().split(' ')[1])
                    while (reader.readLine()?.isNotEmpty() == true) { /* GET headers only. */ }
                    handler(uri, socket.getOutputStream())
                }
            } catch (e: Throwable) { if (!server.isClosed) failure.set(e) }
        }
        try { test("127.0.0.1:${server.localPort}") } finally {
            server.close()
            worker.join(3000)
            failure.get()?.let { throw it }
        }
    }

    @Test fun numericControlPathStillVerifiesSignatureAndIncludesFreshNonce() {
        server({ uri, out ->
            assertEquals("/cgi-bin/mast", uri.path)
            val params = uri.rawQuery.split('&').associate {
                val p = it.split('=', limit = 2)
                p[0] to java.net.URLDecoder.decode(p[1], "UTF-8")
            }
            assertEquals("fw-example", params["device"])
            assertTrue(params.getValue("nonce").matches(Regex("[a-f0-9]{64}")))
            assertEquals(phone.publicKey, params["key"])
            assertTrue(Usign.verify(Usign.parsePublicKey(phone.publicKey), params.getValue("sig"),
                Sync.requestMessage("fw-example", params.getValue("nonce"))))
            reply(out, signed)
        }) { endpoint ->
            assertTrue(Sync.fetch(endpoint, key, "fw-example", phone) is Sync.Result.Ok)
            assertEquals(Sync.Result.Failed("unsupported device policy"),
                Sync.fetch(endpoint, key, "fw-example", phone, requireDevicePolicy = true))
        }
    }

    private val router = PhoneKey.generate() // any usign Ed25519 key can act as a test router

    private fun params(uri: URI) = uri.rawQuery.split('&').associate {
        val p = it.split('=', limit = 2)
        p[0] to java.net.URLDecoder.decode(p[1], "UTF-8")
    }

    /** A signed approved reply of [version] bound to the request in [uri]. */
    private fun signedReply(uri: URI, version: Int): String {
        val q = params(uri)
        val json = """{"v":$version,"server_time":1800000000,"nonce":"${q["nonce"]}","device":"${q["device"]}",
          "enrollment":"approved","binding":{"device_id":"${q["device"]}","profile_id":"kid","profile_name":"Kid",
          "captain":false,"administrator":false,"can_self_remove":false},"captain":"",
          "pact":{"active":false,"type":"","until":0,"release_at":0},
          "rules":{"enabled":"0","policy":"allow","override":"none","windows":[]}
          ${if (version == 3) ""","apps":{"rev":"0123456789abcdef","suspend_new":true,"limits":[],"blocked":[],"approved":[],"downtime":[]}""" else ""}}"""
        return router.sign(json.toByteArray(Charsets.UTF_8)) + "\n" + json
    }

    @Test fun v3RequestSignsTheAppliedRevisionAndAcceptsAV3Reply() {
        server({ uri, out ->
            val q = params(uri)
            val phoneKey = Usign.parsePublicKey(phone.publicKey)
            assertEquals("0123456789abcdef", q["applied"])
            // Old routers check this one and answer v2 ...
            assertTrue(Usign.verify(phoneKey, q.getValue("sig"), Sync.requestMessage("fw-example", q.getValue("nonce"))))
            // ... new routers also check the v3 message, which binds the acknowledgement.
            assertTrue(Usign.verify(phoneKey, q.getValue("sig3"),
                Sync.requestMessageV3("fw-example", q.getValue("nonce"), "0123456789abcdef")))
            assertFalse(Usign.verify(phoneKey, q.getValue("sig3"),
                Sync.requestMessageV3("fw-example", q.getValue("nonce"), "-")))
            reply(out, signedReply(uri, 3))
        }) { endpoint ->
            val r = Sync.fetch(endpoint, router.publicKey, "fw-example", phone, requireDevicePolicy = true,
                appliedRev = "0123456789abcdef", minWire = 3)
            assertEquals("0123456789abcdef", (r as Sync.Result.Ok).state.apps!!.rev)
        }
    }

    @Test fun nothingAppliedIsSentAsDashAndJunkIsNeverAcknowledged() {
        server({ uri, out ->
            assertEquals("-", params(uri)["applied"])
            reply(out, signedReply(uri, 3))
        }) { endpoint ->
            assertTrue(Sync.fetch(endpoint, router.publicKey, "fw-example", phone) is Sync.Result.Ok)
            assertTrue(Sync.fetch(endpoint, router.publicKey, "fw-example", phone, appliedRev = "x&applied=f") is Sync.Result.Ok)
        }
    }

    @Test fun oldRouterStillWorksUntilAV3ReplyWasSeenThenItIsADowngrade() {
        server({ uri, out -> reply(out, signedReply(uri, 2)) }) { endpoint ->
            assertTrue(Sync.fetch(endpoint, router.publicKey, "fw-example", phone, requireDevicePolicy = true, minWire = 2) is Sync.Result.Ok)
            assertEquals(Sync.Result.Failed("downgraded reply"),
                Sync.fetch(endpoint, router.publicKey, "fw-example", phone, requireDevicePolicy = true, minWire = 3))
        }
    }

    @Test fun refusesRedirectRatherThanFollowingCaptivePortal() {
        val redirected = AtomicInteger()
        server({ uri, out ->
            if (uri.path == "/cgi-bin/mast") reply(out, "redirect", 302, "Location: /portal\r\n")
            else { redirected.incrementAndGet(); reply(out, signed) }
        }) { endpoint ->
            assertEquals(Sync.Result.Failed("unexpected HTTP reply"), Sync.fetch(endpoint, key, "fw-example", phone))
            assertEquals(0, redirected.get())
        }
    }

    @Test fun rejectsOversizedRepliesWithoutCallingThemAnOfflineRouter() {
        server({ _, out -> reply(out, "x".repeat(65537)) }) { endpoint ->
            assertEquals(Sync.Result.Failed("reply too large"), Sync.fetch(endpoint, key, "fw-example", phone))
        }
    }

    @Test fun malformedStoredKeyIsAReportedFailureNotAnUncaughtWorkerException() {
        assertEquals(Sync.Result.Failed("bad paired key"), Sync.fetch("127.0.0.1", "not-a-key", "fw-example", phone))
    }
}
