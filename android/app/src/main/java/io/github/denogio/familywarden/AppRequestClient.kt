package io.github.denogio.familywarden

import android.content.Context
import java.io.ByteArrayOutputStream
import java.net.HttpURLConnection
import java.net.URL
import java.util.UUID

/** LAN first. UI choices are never authority: the router rechecks every signed action. */
object AppRequestClient {
    fun request(context: Context, target: String, minutes: Int): UploadReply = synchronized(Mast) {
        val store = Store(context)
        check(Mast.isDeviceOwner(context)) { "managed phone required" }
        check(store.pendingUpload == null) { "pending upload" }
        val fresh = fresh(store)
        val binding = requireNotNull(fresh.state.binding)
        check(fresh.state.apps != null && !Policy.pactActive(fresh.state, fresh.state.serverTime))
        check(fresh.state.request?.let { it.state == "pending" && it.expires > fresh.state.serverTime } != true)
        check(target == "total" || target !in requireNotNull(fresh.state.apps).blocked)
        val label = if (target == "total") "" else runCatching {
            context.packageManager.getApplicationLabel(context.packageManager.getApplicationInfo(target, 0)).toString()
        }.getOrDefault("").let { name ->
            name.substring(0, name.offsetByCodePoints(0, minOf(64, name.codePointCount(0, name.length))))
                .takeIf { it.matches(Regex("[\\p{L}\\p{M}\\p{N}\\p{P}\\p{S} ]{1,64}")) } ?: ""
        }
        val raw = RequestProtocol.request(UUID.randomUUID().toString(), store.deviceId, binding.profileId,
            target, minutes, fresh.state.serverTime, label)
        val upload = RequestUpload("request", raw, requireNotNull(store.publicKey), store.deviceId)
        store.pendingUpload = upload // durable before the first network attempt
        deliver(context, store, fresh, upload)
    }

    fun answer(context: Context, request: ParentRequest, approve: Boolean, minutes: Int): UploadReply = synchronized(Mast) {
        val store = Store(context)
        check(store.pendingUpload == null) { "pending upload" }
        val fresh = fresh(store)
        val current = fresh.state.requests.singleOrNull { it.id == request.id && it.hash == request.hash }
        check(current != null && current.expires > fresh.state.serverTime) { "request no longer pending" }
        check(fresh.state.binding?.approves?.any { it.id == current.profile.id } == true)
        val raw = RequestProtocol.decision(current, store.deviceId, approve, minutes, fresh.state.serverTime)
        val upload = RequestUpload("decision", raw, requireNotNull(store.publicKey), store.deviceId)
        store.pendingUpload = upload
        deliver(context, store, fresh, upload)
    }

    fun retry(context: Context): UploadReply = synchronized(Mast) {
        val store = Store(context)
        val upload = requireNotNull(store.pendingUpload)
        check(upload.routerKey == store.publicKey && upload.device == store.deviceId) { "upload belongs to another pairing" }
        deliver(context, store, fresh(store), upload)
    }

    /** Discarding a local retry does not cancel or reverse any decision at the router. */
    fun forgetRetry(context: Context) = synchronized(Mast) { Store(context).pendingUpload = null }

    private fun fresh(store: Store): Sync.Result.Ok {
        val reply = Sync.fetch(store)
        check(reply is Sync.Result.Ok && reply.state.appliesTo(store.deviceId)) { "verified router required" }
        store.accept(reply)
        return reply
    }

    private fun deliver(context: Context, store: Store, fresh: Sync.Result.Ok, upload: RequestUpload): UploadReply {
        check(upload.routerKey == store.publicKey && upload.device == store.deviceId)
        val reply = post(requireNotNull(store.host), upload, store.phoneKey, fresh.state.serverTime)
        // Only a verified, exact-message-bound answer resolves this outbox entry.
        store.pendingUpload = null
        Mast.refresh(context, sync = true) // grants can arrive ONLY through policy sync
        return reply
    }

    internal fun post(host: String, upload: RequestUpload, phoneKey: PhoneKey, minimumServerTime: Long): UploadReply {
        val body = upload.signedBody(phoneKey)
        val c = URL("${Sync.base(host)}/cgi-bin/app?op=${upload.op}").openConnection() as HttpURLConnection
        c.requestMethod = "POST"
        c.doOutput = true
        c.instanceFollowRedirects = false
        c.connectTimeout = 5000
        c.readTimeout = 5000
        c.useCaches = false
        c.setRequestProperty("Content-Type", "text/plain; charset=utf-8")
        c.setFixedLengthStreamingMode(body.size)
        try {
            c.outputStream.use { it.write(body) }
            check(c.responseCode == 200) { "router did not accept upload" }
            val bytes = c.inputStream.use { input ->
                val out = ByteArrayOutputStream()
                val buffer = ByteArray(4096)
                while (true) {
                    val n = input.read(buffer)
                    if (n < 0) break
                    require(out.size() + n <= 65536 + 101) { "upload reply too large" }
                    out.write(buffer, 0, n)
                }
                out.toByteArray()
            }
            return RequestProtocol.verifyReply(bytes, upload, minimumServerTime)
        } finally { c.disconnect() }
    }
}
