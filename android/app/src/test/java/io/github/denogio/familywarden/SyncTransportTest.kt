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
