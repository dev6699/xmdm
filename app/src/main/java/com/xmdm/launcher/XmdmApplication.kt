package com.xmdm.launcher

import android.app.Application
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import com.xmdm.launcher.kiosk.KioskAdminOverlayController

class XmdmApplication : Application() {
    val appScope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    val kioskAdminOverlayController by lazy { KioskAdminOverlayController(this) }
}
