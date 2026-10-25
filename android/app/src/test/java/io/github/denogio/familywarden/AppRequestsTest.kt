package io.github.denogio.familywarden

import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class AppRequestsTest {
    private val now = 1800000000L
    private val router = PhoneKey.generate()
    private val phone = PhoneKey.generate()
    private fun rejected(block: () -> Unit) {
        try { block(); fail("invalid authority accepted") } catch (_: Exception) { }
    }
    private fun upload(op: String = "request"): RequestUpload {
        val raw = if (op == "request") RequestProtocol.request("request-001", "fw-alex", "alex", "total", 30, now)
        else RequestProtocol.decision(parent(), "fw-parent", true, 20, now)
        return RequestUpload(op, raw, router.publicKey, if (op == "request") "fw-alex" else "fw-parent")
    }
    private fun response(u: RequestUpload) = JSONObject().put("v", 1).put("type", "${u.op}_reply")
        .put("device", u.device).put("server_time", now).put("message_sha256", RequestProtocol.digest(u.raw.toByteArray()))
        .put("request_id", "request-001").put("state", "pending")
    private fun signed(o: JSONObject): ByteArray = o.toString().toByteArray().let { raw ->
        (router.sign(raw) + "\n").toByteArray() + raw
    }
    private fun grant() = JSONObject().put("grant_id", "a".repeat(32)).put("device_id", "fw-alex")
        .put("target", "com.example.video").put("minutes", 20).put("not_before", now).put("not_after", now + 7200)
    private fun parent() = ParentRequest("request-001", "b".repeat(64), ProfileRef("alex", "Alex"),
        "com.example.video", 30, now, now + 3600)
    private fun parentJson() = JSONObject().put("request_id", "request-001").put("request_sha256", "b".repeat(64))
        .put("profile_id", "alex").put("profile_name", "Alex").put("target", "total").put("minutes", 30)
        .put("received", now).put("expires", now + 3600).put("label", "Screen time")

    @Test fun persistentUploadKeepsExactUnicodeBytesAndSigningDomain() {
        val u = RequestUpload("request", RequestProtocol.request("request-001", "fw-alex", "alex", "com.example.video", 30, now,
            "Video på dansk"), router.publicKey, "fw-alex")
        val restored = RequestUpload.decode(u.encode())
        assertEquals(u, restored)
        assertArrayEquals(u.signedBody(phone), restored.signedBody(phone))
        val body = restored.signedBody(phone)
        val signature = body.copyOfRange(0, 100).toString(Charsets.US_ASCII)
        assertTrue(Usign.verify(Usign.parsePublicKey(phone.publicKey), signature,
            "familywarden-request-v1\n".toByteArray() + restored.raw.toByteArray()))
        assertFalse(Usign.verify(Usign.parsePublicKey(phone.publicKey), signature,
            "familywarden-decision-v1\n".toByteArray() + restored.raw.toByteArray()))
    }

    @Test fun uploadReplyHasStatusButNoPolicyAuthority() {
        val u = upload()
        assertEquals(UploadReply("pending", null), RequestProtocol.verifyReply(signed(response(u)), u, now))
        val o = response(u).put("binding", JSONObject().put("can_self_remove", true))
        assertEquals(UploadReply("pending", null), RequestProtocol.verifyReply(signed(o), u, now))
    }

    @Test fun authenticReplyStillMustMatchDeviceMessageTypeHashAndClock() {
        val u = upload()
        for ((key, value) in listOf("device" to "fw-other", "type" to "decision_reply", "message_sha256" to "c".repeat(64),
            "request_id" to "another-001", "server_time" to now - 1, "v" to 3)) {
            rejected { RequestProtocol.verifyReply(signed(response(u).put(key, value)), u, now) }
        }
        val wrongPin = u.copy(routerKey = phone.publicKey)
        rejected { RequestProtocol.verifyReply(signed(response(u)), wrongPin, now) }
        val tampered = signed(response(u)).also { it[it.lastIndex - 2] = 'x'.code.toByte() }
        rejected { RequestProtocol.verifyReply(tampered, u, now) }
    }

    @Test fun signedErrorsAreTypedAndBoundToTheExactUpload() {
        val u = upload()
        for (error in listOf("request_open", "not_parent", "already_answered", "unavailable")) {
            assertEquals(error, RequestProtocol.verifyReply(signed(response(u).put("error", error)), u, now).error)
        }
        rejected { RequestProtocol.verifyReply(signed(response(u).put("error", "<script>")), u, now) }
    }

    @Test fun decisionReplyCannotWidenMinutesOrLifetime() {
        val u = upload("decision")
        val o = response(u).put("state", "approved").put("granted", 20).put("not_after", now + 7200)
            .put("home_internet", true)
        assertEquals(UploadReply("approved", null, 20, now + 7200, true), RequestProtocol.verifyReply(signed(o), u, now))
        rejected { RequestProtocol.verifyReply(signed(JSONObject(o.toString()).put("granted", 21)), u, now) }
        rejected { RequestProtocol.verifyReply(signed(JSONObject(o.toString()).put("not_after", now + 7201)), u, now) }
        rejected { RequestProtocol.verifyReply(signed(JSONObject(o.toString()).put("granted", "20")), u, now) }
        rejected { RequestProtocol.verifyReply(signed(JSONObject(o.toString()).put("home_internet", "true")), u, now) }
    }

    @Test fun grantsAreDeviceBoundUniqueAndBounded() {
        val a = JSONObject().put("grants", JSONArray().put(grant()))
        assertEquals(listOf(AppGrant("a".repeat(32), "fw-alex", "com.example.video", 20, now, now + 7200)),
            RequestProtocol.grants(a, "fw-alex"))
        rejected { RequestProtocol.grants(a, "fw-other") }
        rejected { RequestProtocol.grants(JSONObject().put("grants", JSONArray().put(grant()).put(grant())), "fw-alex") }
        rejected { RequestProtocol.grants(JSONObject().put("grants", JSONArray((0..32).map { grant().put("grant_id", "%032x".format(it)) })), "fw-alex") }
        for ((key, value) in listOf("minutes" to 0, "minutes" to 241, "minutes" to "20", "target" to "../video",
            "not_before" to -1, "not_after" to now + 7201, "not_after" to now, "grant_id" to "not-an-id")) {
            rejected { RequestProtocol.grants(JSONObject().put("grants", JSONArray().put(grant().put(key, value))), "fw-alex") }
        }
        assertTrue(RequestProtocol.grants(JSONObject(), "fw-alex").isEmpty())
        rejected { RequestProtocol.grants(JSONObject().put("grants", JSONObject.NULL), "fw-alex") }
    }

    @Test fun parentRequestsRequireCurrentSignedScope() {
        val a = JSONArray().put(parentJson())
        assertEquals("alex", RequestProtocol.parents(a, listOf(ProfileRef("alex", "Alex"))).single().profile.id)
        rejected { RequestProtocol.parents(a, emptyList()) }
        rejected { RequestProtocol.parents(a, listOf(ProfileRef("other", "Other"))) }
        rejected { RequestProtocol.parents(JSONArray().put(parentJson()).put(parentJson()), listOf(ProfileRef("alex", "Alex"))) }
        rejected { RequestProtocol.parents(JSONArray().put(parentJson().put("request_sha256", "")), listOf(ProfileRef("alex", "Alex"))) }
        rejected { RequestProtocol.parents(JSONArray().put(parentJson().put("expires", now + 3601)), listOf(ProfileRef("alex", "Alex"))) }
    }

    @Test fun childStatusIncludesDenialExpiryAndApprovalWithoutImplyingConsumption() {
        val o = JSONObject().put("request_id", "request-001").put("target", "total").put("minutes", 30).put("expires", now + 3600)
        for (state in listOf("pending", "denied", "expired")) assertEquals(state, RequestProtocol.child(o.put("state", state)).state)
        o.put("state", "approved").put("granted", 20).put("not_after", now + 7200).put("answered_by", "Taylor")
        assertEquals(20, RequestProtocol.child(o).granted)
        rejected { RequestProtocol.child(JSONObject(o.toString()).put("granted", 31)) }
        rejected { RequestProtocol.child(JSONObject(o.toString()).put("state", "unlimited")) }
    }
}
