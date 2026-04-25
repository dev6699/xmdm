package com.xmdm.launcher.apps

import android.content.pm.PackageInstaller
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CompletableDeferred

internal object ManagedAppInstallResultRegistry {
    private val pending = ConcurrentHashMap<String, CompletableDeferred<Unit>>()

    fun register(action: String): CompletableDeferred<Unit> {
        val deferred = CompletableDeferred<Unit>()
        pending[action] = deferred
        return deferred
    }

    fun complete(action: String, resultCode: Int, message: String?) {
        pending.remove(action)?.let { deferred ->
            if (resultCode == PackageInstaller.STATUS_SUCCESS) {
                deferred.complete(Unit)
            } else {
                val failureMessage = buildString {
                    append("managed app operation failed")
                    append(" (code=")
                    append(resultCode)
                    if (!message.isNullOrBlank()) {
                        append(", message=")
                        append(message)
                    }
                    append(')')
                }
                deferred.completeExceptionally(IllegalStateException(failureMessage))
            }
        }
    }

    fun clear(action: String) {
        pending.remove(action)
    }
}
