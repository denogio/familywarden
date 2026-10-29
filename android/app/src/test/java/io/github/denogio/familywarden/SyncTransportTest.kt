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

    @Test fun routerAdministrationUsesTheHostNotTheSailPortAndRejectsAddressInjection() {
        val path = "/"
        assertEquals("http://192.0.2.1$path", Sync.routerAdminUrl("192.0.2.1"))
        assertEquals("http://router.test$path", Sync.routerAdminUrl("router.test:7700"))
        assertEquals("http://[2001:db8::1]$path", Sync.routerAdminUrl("[2001:db8::1]:7701"))
        for (bad in listOf("user@router.test", "router.test/admin", "router.test?next=evil", "router.test#fragment", "router.test:0")) {
            assertTrue(bad, runCatching { Sync.routerAdminUrl(bad) }.isFailure)
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
    private fun signedReply(uri: URI, version: Int, change: (org.json.JSONObject) -> Unit = {}): String {
        val q = params(uri)
        val json = """{"v":$version,"server_time":1800000000,"nonce":"${q["nonce"]}","device":"${q["device"]}",
          "enrollment":"approved","binding":{"device_id":"${q["device"]}","profile_id":"kid","profile_name":"Kid",
          "captain":false,"administrator":false,"can_self_remove":false},"captain":"",
          "pact":{"active":false,"type":"","until":0,"release_at":0},
          "rules":{"enabled":"0","policy":"allow","override":"none","windows":[]}
          ${if (version == 3) ""","apps":{"rev":"0123456789abcdef","suspend_new":true,"limits":[],"blocked":[],"approved":[],"downtime":[]}""" else ""}}"""
        val payload = org.json.JSONObject(json).also(change).toString()
        return router.sign(payload.toByteArray(Charsets.UTF_8)) + "\n" + payload
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
            assertEquals("apps-v1,grants-v1,screen-time-v1,shared-v1", q["caps"])
            assertTrue(Usign.verify(phoneKey, q.getValue("sigcaps"),
                Sync.requestCapabilitiesMessage("fw-example", q.getValue("nonce"), "0123456789abcdef", q.getValue("caps"))))
            assertFalse(Usign.verify(phoneKey, q.getValue("sigcaps"),
                Sync.requestCapabilitiesMessage("fw-example", q.getValue("nonce"), "-", q.getValue("caps"))))
            assertFalse(Usign.verify(phoneKey, q.getValue("sigcaps"),
                Sync.requestCapabilitiesMessage("fw-example", q.getValue("nonce"), "0123456789abcdef", "-")))
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

    @Test fun signedUnknownRequirementsAreReportedWithoutAcceptingANewPolicy() {
        server({ uri, out -> reply(out, signedReply(uri, 3) {
            it.put("requires", org.json.JSONArray().put("future-parent-approval"))
        }) }) { endpoint ->
            assertEquals(Sync.Result.Failed("unsupported capabilities"),
                Sync.fetch(endpoint, router.publicKey, "fw-example", phone, requireDevicePolicy = true))
        }
    }

    @Test fun signedCapabilityRefusalsMustMatchThisRequestAndCannotAuthorizeRemoval() {
        val wrongNonce = java.util.concurrent.atomic.AtomicBoolean(false)
        server({ uri, out -> reply(out, signedReply(uri, 3) {
            it.put("requires", org.json.JSONArray().put("screen-time-v1"))
            it.put("error", "unsupported_capabilities")
            it.remove("binding")
            if (wrongNonce.get()) it.put("nonce", "b".repeat(64))
        }) }) { endpoint ->
            assertEquals(Sync.Result.Failed("unsupported capabilities"),
                Sync.fetch(endpoint, router.publicKey, "fw-example", phone, requireDevicePolicy = true))
            wrongNonce.set(true)
            assertEquals(Sync.Result.Failed("response mismatch"),
                Sync.fetch(endpoint, router.publicKey, "fw-example", phone, requireDevicePolicy = true))
        }
    }

    @Test fun understoodRequirementsRemainCompatibleWithNormalV3Policy() {
        server({ uri, out -> reply(out, signedReply(uri, 3) {
            it.put("requires", org.json.JSONArray().put("screen-time-v1"))
        }) }) { endpoint ->
            assertTrue(Sync.fetch(endpoint, router.publicKey, "fw-example", phone, requireDevicePolicy = true) is Sync.Result.Ok)
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
    @Test fun parentOnlyClientAuthenticatesThatItDoesNotEnforceManagedApps() {
        server({ uri, out ->
            val q = params(uri)
            assertEquals("-", q["caps"])
            assertTrue(Usign.verify(Usign.parsePublicKey(phone.publicKey), q.getValue("sigcaps"),
                Sync.requestCapabilitiesMessage("fw-parent", q.getValue("nonce"), "-", "-")))
            reply(out, signedReply(uri, 3) {
                it.remove("apps")
                it.getJSONObject("binding").put("profile_id", "parent").put("profile_name", "Taylor")
                    .put("approves", org.json.JSONArray().put(org.json.JSONObject().put("profile_id", "kid").put("profile_name", "Alex")))
            })
        }) { endpoint ->
            val result = Sync.fetch(endpoint, router.publicKey, "fw-parent", phone, capabilities = "-")
            assertTrue(result is Sync.Result.Ok && result.state.parentOnlyCompatible())
        }
    }

    @Test fun aRestoredOlderHouseholdKeepsTheNewerTrustedPolicy() {
        val epoch = "a".repeat(32)
        val restored = "b".repeat(32)
        fun generation(e: String, counter: Long, vararg lineage: Pair<String, Long>) = org.json.JSONObject()
            .put("epoch", e).put("counter", counter).put("lineage", org.json.JSONArray().also { a ->
                lineage.forEach { (le, lc) -> a.put(org.json.JSONObject().put("epoch", le).put("counter", lc)) }
            })
        val next = AtomicReference<org.json.JSONObject?>(generation(epoch, 5))
        server({ uri, out -> reply(out, signedReply(uri, 3) { o -> next.get()?.let { o.put("generation", it) } }) }) { endpoint ->
            fun fetch(previous: MastState?) = Sync.fetch(endpoint, router.publicKey, "fw-example", phone,
                previous = previous, requireDevicePolicy = true, minWire = 3)
            val seen = (fetch(null) as Sync.Result.Ok).state
            assertEquals(HouseholdGeneration(epoch, 5, emptyList()), seen.generation)
            val older = Sync.Result.Failed("router state is older than this phone")
            next.set(generation(epoch, 4)); assertEquals(older, fetch(seen))
            // An older backup on a replacement router, even after many later writes.
            next.set(generation(restored, 900, epoch to 4)); assertEquals(older, fetch(seen))
            // A router that stops vouching is not newer either.
            next.set(null); assertEquals(older, fetch(seen))
            // The newest backup restored: same key, fork point covers this phone.
            next.set(generation(restored, 0, epoch to 5))
            val after = (fetch(seen) as Sync.Result.Ok).state
            next.set(generation(restored, 1, epoch to 5)); assertTrue(fetch(after) is Sync.Result.Ok)
        }
    }

}
