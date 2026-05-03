package com.xmdm.launcher

import android.app.admin.DevicePolicyManager
import android.content.BroadcastReceiver
import android.content.ComponentName
import android.content.Context
import android.content.Intent

class ClearAdminReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val dpm = context.getSystemService(Context.DEVICE_POLICY_SERVICE) as DevicePolicyManager
        val component = ComponentName(context, AdminReceiver::class.java)
        dpm.clearDeviceOwnerApp(context.packageName)
        dpm.removeActiveAdmin(component)
    }
}