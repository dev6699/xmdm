package com.xmdm.launcher.apps

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.util.Log

class ManagedAppInstallResultReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val action = intent.action ?: return
        val resultCode = intent.getIntExtra(
            PackageInstaller.EXTRA_STATUS,
            PackageInstaller.STATUS_FAILURE,
        )
        val message = intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE)
        ManagedAppInstallResultRegistry.complete(action, resultCode, message)
        Log.i(TAG, "managed app operation completed: $action code=$resultCode")
    }

    companion object {
        private const val TAG = "XmdmLauncher"
    }
}
