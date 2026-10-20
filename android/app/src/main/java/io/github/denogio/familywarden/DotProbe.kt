package io.github.denogio.familywarden

import android.net.Network
import io.github.denogio.familywarden.ui.ProbeResult
import java.io.DataInputStream
import java.io.IOException
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Socket
import java.security.SecureRandom
import java.security.cert.CertificateException
import javax.net.ssl.SNIHostName
import javax.net.ssl.SSLHandshakeException
import javax.net.ssl.SSLPeerUnverifiedException
import javax.net.ssl.SSLSocket
import javax.net.ssl.SSLSocketFactory

/**
 * One DNS-over-TLS query to the policy's resolver on the given network: does it
 * answer over a verified TLS connection? This tests the resolver path only, not
 * that every filter category works.
 */
object DotProbe {
    private const val PORT = 853
    private const val TIMEOUT_MS = 5000

    fun probe(network: Network, hostname: String): ProbeResult {
        val id = SecureRandom().nextInt(0x10000)
        return try {
            val address = network.getAllByName(hostname).first()
            network.socketFactory.createSocket().use { plain ->
                plain.connect(InetSocketAddress(address, PORT), TIMEOUT_MS)
                plain.soTimeout = TIMEOUT_MS
                val factory = SSLSocketFactory.getDefault() as SSLSocketFactory
                (factory.createSocket(plain, hostname, PORT, true) as SSLSocket).use { tls ->
                tls.sslParameters = tls.sslParameters.apply {
                    endpointIdentificationAlgorithm = "HTTPS"
                    serverNames = listOf(SNIHostName(hostname))
                }
                tls.startHandshake()
                val q = query(id)
                tls.outputStream.apply { write(byteArrayOf((q.size shr 8).toByte(), q.size.toByte())); write(q); flush() }
                val input = DataInputStream(tls.inputStream)
                val length = input.readUnsignedShort()
                if (length < 12 || length > 65535) return ProbeResult.DNS_ERROR
                check(ByteArray(length).also { input.readFully(it) }, id)
                }
            }
        } catch (e: SSLHandshakeException) {
            if (generateSequence<Throwable>(e) { it.cause }.any { it is CertificateException })
                ProbeResult.CERTIFICATE else ProbeResult.UNAVAILABLE
        } catch (e: SSLPeerUnverifiedException) {
            ProbeResult.CERTIFICATE
        } catch (e: IOException) {
            ProbeResult.UNAVAILABLE
        } catch (e: RuntimeException) {
            ProbeResult.UNAVAILABLE
        }
    }

    /** Recursive A query for example.com (RFC 1035 wire format). */
    internal fun query(id: Int): ByteArray {
        val name = "example.com".split('.').flatMap { listOf(it.length.toByte()) + it.toByteArray().toList() } + 0.toByte()
        return byteArrayOf((id shr 8).toByte(), id.toByte(), 0x01, 0x00, 0, 1, 0, 0, 0, 0, 0, 0) +
            name.toByteArray() + byteArrayOf(0, 1, 0, 1)
    }

    /** A non-truncated standard answer echoing our question; NOERROR or NXDOMAIN. */
    internal fun check(response: ByteArray, id: Int): ProbeResult {
        val question = query(id)
        if (response.size < question.size) return ProbeResult.DNS_ERROR
        val rid = ((response[0].toInt() and 0xff) shl 8) or (response[1].toInt() and 0xff)
        val flags = response[2].toInt() and 0xff
        val rcode = response[3].toInt() and 0x0f
        val valid = rid == id && flags and 0x80 != 0 && flags and 0x7a == 0 &&
            response[4].toInt() == 0 && response[5].toInt() == 1 &&
            response.copyOfRange(12, question.size).contentEquals(question.copyOfRange(12, question.size))
        return if (valid && rcode in setOf(0, 3)) ProbeResult.ANSWERED else ProbeResult.DNS_ERROR
    }
}
