package io.github.denogio.familywarden

/** An authenticated receipt plus Android's monotonic clock. No wall-clock grant authority. */
data class PolicyClock(val serverMillis: Long, val elapsedMillis: Long, val boot: Int, val nonce: String) {
    init {
        require(serverMillis in 1..253402300799000L && elapsedMillis >= 0 && boot >= 0)
        require(nonce.matches(Regex("[0-9a-f]{64}")))
    }
    companion object {
        /** A faster subsequent reply must not move the running trusted clock back. */
        fun receipt(serverEpoch: Long, started: Long, received: Long, boot: Int, nonce: String,
            previous: PolicyClock? = null): PolicyClock {
            require(serverEpoch in 1..253402300799L && started in 0..received)
            val upper = Math.addExact(serverEpoch * 1000, received - started)
            return PolicyClock(maxOf(upper, previous?.now(received, boot) ?: 0), received, boot, nonce)
        }
    }

    fun now(elapsed: Long, currentBoot: Int): Long? {
        if (currentBoot != boot || elapsed < elapsedMillis) return null
        val delta = elapsed - elapsedMillis
        return if (delta <= 253402300799000L - serverMillis) serverMillis + delta else null
    }
}
