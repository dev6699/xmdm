package com.xmdm.launcher

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log

class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (intent.action) {
            Intent.ACTION_BOOT_COMPLETED,
            Intent.ACTION_LOCKED_BOOT_COMPLETED,
            -> {
                runCatching {
                    val launchIntent = MainActivity.intent(context).apply {
                        addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP)
                        putExtra(MainActivity.EXTRA_STARTED_FROM_BOOT, true)
                    }
                    context.startActivity(launchIntent)
                }.onFailure {
                    Log.w(TAG, "failed to start launcher after boot", it)
                }
            }
        }
    }

    companion object {
        private const val TAG = "XmdmLauncher"
    }
}
