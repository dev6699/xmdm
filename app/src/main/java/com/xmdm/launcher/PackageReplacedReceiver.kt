package com.xmdm.launcher

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log

class PackageReplacedReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_MY_PACKAGE_REPLACED) {
            return
        }
        runCatching {
            val launchIntent = MainActivity.intent(context).apply {
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP)
            }
            context.startActivity(launchIntent)
        }.onFailure {
            Log.w(TAG, "failed to relaunch launcher after package replacement", it)
        }
    }

    companion object {
        private const val TAG = "XmdmLauncher"
    }
}
