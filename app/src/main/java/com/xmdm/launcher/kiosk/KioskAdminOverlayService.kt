package com.xmdm.launcher.kiosk

import android.app.Service
import android.content.Intent
import android.os.IBinder
import com.xmdm.launcher.state.AgentStateStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

class KioskAdminOverlayService : Service() {
    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val stateStore by lazy { AgentStateStore.from(applicationContext) }
    private val overlayController by lazy { KioskAdminOverlayController(applicationContext) }
    private var stateJob: Job? = null

    override fun onCreate() {
        super.onCreate()
        stateJob = serviceScope.launch {
            stateStore.state.collectLatest { state ->
                overlayController.update(state.isBootstrapped && state.isEnrolled)
            }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        return START_STICKY
    }

    override fun onDestroy() {
        stateJob?.cancel()
        stateJob = null
        overlayController.hide()
        serviceScope.cancel()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null
}
