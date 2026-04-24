package com.xmdm.launcher.retry

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class RetryRunnerTest {
    @Test
    fun retriesWithExponentialBackoffAndEventuallySucceeds() = runTest {
        val sleeps = mutableListOf<Long>()
        val result = retrying(
            policy = RetryPolicy(maxAttempts = 4, initialDelayMs = 100, maxDelayMs = 1000),
            sleeper = object : Sleeper {
                override suspend fun sleep(durationMs: Long) {
                    sleeps += durationMs
                }
            },
        ) { attempt ->
            if (attempt < 3) {
                error("transient failure")
            }
            "ok"
        }

        assertEquals("ok", result)
        assertEquals(listOf(100L, 200L), sleeps)
    }

    @Test
    fun stopsAfterMaxAttempts() = runTest {
        val sleeps = mutableListOf<Long>()
        var failure: IllegalStateException? = null

        try {
            retrying(
                policy = RetryPolicy(maxAttempts = 3, initialDelayMs = 50, maxDelayMs = 200),
                sleeper = object : Sleeper {
                    override suspend fun sleep(durationMs: Long) {
                        sleeps += durationMs
                    }
                },
            ) {
                error("still failing")
            }
        } catch (caught: IllegalStateException) {
            failure = caught
        }

        if (failure == null) {
            fail("expected retrying to throw")
        }

        assertEquals("still failing", failure?.message)
        assertEquals(listOf(50L, 100L), sleeps)
    }
}
