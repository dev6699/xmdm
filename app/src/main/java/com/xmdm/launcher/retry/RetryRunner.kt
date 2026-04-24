package com.xmdm.launcher.retry

import kotlinx.coroutines.delay

interface Sleeper {
    suspend fun sleep(durationMs: Long)
}

object CoroutineSleeper : Sleeper {
    override suspend fun sleep(durationMs: Long) {
        delay(durationMs)
    }
}

data class RetryPolicy(
    val maxAttempts: Int = 4,
    val initialDelayMs: Long = 500,
    val maxDelayMs: Long = 30_000,
    val multiplier: Double = 2.0,
) {
    init {
        require(maxAttempts >= 1) { "maxAttempts must be at least 1" }
        require(initialDelayMs >= 0) { "initialDelayMs must be non-negative" }
        require(maxDelayMs >= initialDelayMs) { "maxDelayMs must be >= initialDelayMs" }
        require(multiplier >= 1.0) { "multiplier must be at least 1.0" }
    }

    fun delayForAttempt(attempt: Int): Long {
        require(attempt >= 1) { "attempt must be at least 1" }
        var delay = initialDelayMs.toDouble()
        repeat(attempt - 1) {
            delay = (delay * multiplier).coerceAtMost(maxDelayMs.toDouble())
        }
        return delay.toLong().coerceAtMost(maxDelayMs)
    }
}

suspend fun <T> retrying(
    policy: RetryPolicy = RetryPolicy(),
    sleeper: Sleeper = CoroutineSleeper,
    block: suspend (attempt: Int) -> T,
): T {
    var attempt = 1
    var lastFailure: Throwable? = null

    while (attempt <= policy.maxAttempts) {
        try {
            return block(attempt)
        } catch (failure: Throwable) {
            lastFailure = failure
            if (attempt >= policy.maxAttempts) {
                break
            }
            sleeper.sleep(policy.delayForAttempt(attempt))
            attempt += 1
        }
    }

    throw lastFailure ?: IllegalStateException("retrying failed without a recorded error")
}
