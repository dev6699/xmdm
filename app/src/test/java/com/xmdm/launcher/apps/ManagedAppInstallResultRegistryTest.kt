package com.xmdm.launcher.apps

import android.content.pm.PackageInstaller
import kotlinx.coroutines.ExperimentalCoroutinesApi
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ManagedAppInstallResultRegistryTest {
    @Test
    fun completesPendingResultOnSuccess() {
        val action = "install:com.example.app:1"
        val deferred = ManagedAppInstallResultRegistry.register(action)

        ManagedAppInstallResultRegistry.complete(action, PackageInstaller.STATUS_SUCCESS, null)

        assertTrue(deferred.isCompleted)
        assertEquals(Unit, deferred.getCompleted())
    }

    @Test
    fun completesPendingResultExceptionallyOnFailure() {
        val action = "install:com.example.app:2"
        val deferred = ManagedAppInstallResultRegistry.register(action)

        ManagedAppInstallResultRegistry.complete(action, PackageInstaller.STATUS_FAILURE, "out of space")

        assertTrue(deferred.isCompleted)
        val failure = deferred.getCompletionExceptionOrNull()
        assertTrue(failure is IllegalStateException)
        assertEquals(
            "managed app operation failed (code=${PackageInstaller.STATUS_FAILURE}, message=out of space)",
            failure?.message,
        )
    }
}
