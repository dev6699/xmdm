package com.xmdm.launcher

import android.os.Bundle
import android.app.admin.DevicePolicyManager
import android.util.Base64
import android.util.Log
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import com.google.gson.GsonBuilder
import com.google.gson.JsonParser
import com.xmdm.launcher.bootstrap.BootstrapProvisioner
import com.xmdm.launcher.apps.AndroidManagedAppInstaller
import com.xmdm.launcher.apps.HttpManagedAppDownloader
import com.xmdm.launcher.apps.ManagedAppInstallCoordinator
import com.xmdm.launcher.apps.ManagedAppInstallProgress
import com.xmdm.launcher.databinding.ActivityMainBinding
import com.xmdm.launcher.enrollment.EnrollmentCoordinator
import com.xmdm.launcher.enrollment.HttpEnrollmentGateway
import com.xmdm.launcher.recovery.RecoveryActivity
import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.state.AgentStateStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import kotlin.math.max
import kotlin.math.min

class MainActivity : AppCompatActivity() {
    private val stateStore by lazy { AgentStateStore.from(applicationContext) }
    private val enrollmentCoordinator by lazy {
        EnrollmentCoordinator(
            stateStore = stateStore,
            gateway = HttpEnrollmentGateway(),
        )
    }
    private val managedAppCoordinator by lazy {
        ManagedAppInstallCoordinator(
            downloader = HttpManagedAppDownloader(),
            installer = AndroidManagedAppInstaller(applicationContext),
        )
    }
    private val managedAppProgress = MutableStateFlow<ManagedAppInstallProgress>(ManagedAppInstallProgress.Idle)
    private lateinit var binding: ActivityMainBinding
    private var enrollmentInFlight = false
    private var appInstallInFlight = false
    private var lastManagedAppsSnapshotVersion: Long? = null
    private val prettyJson = GsonBuilder().setPrettyPrinting().create()
    private var cachedPrettySnapshotJson: String? = null
    private var cachedPrettySnapshotText: String = ""
    private var lastProvisioningRunId: String? = null
    private var recoveryVisible = false
    private var latestState: AgentState = AgentState.empty()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)

        binding.launcherTitle.text = getString(R.string.launcher_title)
        lifecycleScope.launch {
            consumeBootstrapIntent()
            stateStore.state.collectLatest { state ->
                latestState = state
                renderLauncherStatus()
                maybeStartEnrollment(state)
                maybeApplyManagedApps(state)
            }
        }
        lifecycleScope.launch {
            managedAppProgress.collectLatest {
                renderManagedAppProgress()
            }
        }
    }

    override fun onNewIntent(intent: android.content.Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        lifecycleScope.launch {
            consumeBootstrapIntent()
        }
    }

    private fun renderUi() {
        renderManagedAppProgress()
        renderLauncherStatus()
    }

    private fun renderManagedAppProgress() {
        binding.launcherActivity.text = renderLiveManagedAppStatus(managedAppProgress.value)
    }

    private fun renderLauncherStatus() {
        binding.launcherStatus.text = renderStatus(latestState, enrollmentInFlight)
    }

    private fun renderStatus(state: AgentState, enrollmentInFlight: Boolean): CharSequence {
        val bootstrapLine = if (state.isBootstrapped) {
            "bootstrap: restored"
        } else {
            "bootstrap: empty"
        }
        val identityLine = when {
            state.isEnrolled -> "device identity: restored"
            enrollmentInFlight -> "device identity: enrolling"
            else -> "device identity: empty"
        }
        val deviceOwnerLine = if (isDeviceOwnerApp()) {
            "device owner: yes"
        } else {
            "device owner: no"
        }
        val policyLine = if (state.hasPolicyCache) {
            val policyCache = state.policyCache!!
            val savedAt = Instant.ofEpochMilli(policyCache.lastSyncAtEpochMillis)
            buildString {
                append("policy cache: restored")
                append('\n')
                append("saved at: ")
                append(formatSavedAt(savedAt))
                append('\n')
                append("config snapshot:")
                append('\n')
                append(prettyConfigCached(policyCache.snapshotJson))
            }
        } else {
            "policy cache: empty\nsaved at: -\nconfig snapshot: empty"
        }
        val managedAppsLine = if (state.hasManagedApps) {
            "managed apps: restored"
        } else {
            "managed apps: empty"
        }
        return buildString {
            append(getString(R.string.launcher_status))
            append('\n')
            append(bootstrapLine)
            append('\n')
            append(identityLine)
            append('\n')
            append(deviceOwnerLine)
            append('\n')
            append(policyLine)
            append('\n')
            append(managedAppsLine)
        }
    }

    private fun renderLiveManagedAppStatus(progress: ManagedAppInstallProgress): CharSequence {
        return when (progress) {
            ManagedAppInstallProgress.Idle -> getString(R.string.launcher_live_idle)
            ManagedAppInstallProgress.VerifyingSnapshot -> getString(R.string.launcher_live_verifying)
            is ManagedAppInstallProgress.Downloading -> {
                val appName = progress.app.name ?: progress.app.packageName
                val totalBytes = progress.totalBytes
                if (totalBytes != null && totalBytes > 0) {
                    getString(
                        R.string.launcher_live_downloading,
                        appName,
                        progress.app.versionName,
                        progress.index,
                        progress.total,
                        formatBytes(progress.downloadedBytes),
                        formatBytes(totalBytes),
                        percent(progress.downloadedBytes, totalBytes),
                    )
                } else {
                    getString(
                        R.string.launcher_live_downloading_unknown_total,
                        appName,
                        progress.app.versionName,
                        progress.index,
                        progress.total,
                        formatBytes(progress.downloadedBytes),
                    )
                }
            }
            is ManagedAppInstallProgress.Installing -> getString(
                R.string.launcher_live_installing,
                progress.app.name ?: progress.app.packageName,
                progress.app.versionName,
                progress.index,
                progress.total,
            )
            is ManagedAppInstallProgress.Uninstalling -> getString(
                R.string.launcher_live_uninstalling,
                progress.packageName,
                progress.index,
                progress.total,
            )
            is ManagedAppInstallProgress.Queued -> getString(
                R.string.launcher_live_queued,
                progress.installed.size,
                progress.uninstalled.size,
            )
            is ManagedAppInstallProgress.Failed -> getString(
                R.string.launcher_live_failed,
                progress.message,
            )
        }
    }

    private fun isDeviceOwnerApp(): Boolean {
        val devicePolicyManager = getSystemService(DevicePolicyManager::class.java) ?: return false
        return devicePolicyManager.isDeviceOwnerApp(packageName)
    }

    private fun prettyConfig(snapshotJson: String): String {
        val parsed = JsonParser.parseString(snapshotJson)
        return prettyJson.toJson(parsed)
    }

    private fun prettyConfigCached(snapshotJson: String): String {
        if (cachedPrettySnapshotJson == snapshotJson) {
            return cachedPrettySnapshotText
        }
        val formatted = prettyConfig(snapshotJson)
        cachedPrettySnapshotJson = snapshotJson
        cachedPrettySnapshotText = formatted
        return formatted
    }

    private fun formatSavedAt(instant: Instant): String {
        return SAVED_AT_FORMATTER.format(instant.atZone(ZoneId.systemDefault()))
    }

    private fun formatBytes(bytes: Long): String {
        val clamped = max(bytes, 0L).toDouble()
        val units = arrayOf("B", "KB", "MB", "GB", "TB")
        var value = clamped
        var unitIndex = 0
        while (value >= 1024.0 && unitIndex < units.lastIndex) {
            value /= 1024.0
            unitIndex += 1
        }
        return if (unitIndex == 0) {
            "${value.toLong()} ${units[unitIndex]}"
        } else {
            String.format("%.1f %s", value, units[unitIndex])
        }
    }

    private fun percent(downloadedBytes: Long, totalBytes: Long): Int {
        if (totalBytes <= 0) {
            return 0
        }
        val ratio = (downloadedBytes.toDouble() / totalBytes.toDouble()) * 100.0
        return min(100, ratio.toInt())
    }

    private fun maybeStartEnrollment(state: AgentState) {
        if (enrollmentInFlight) {
            return
        }
        val bootstrap = state.bootstrap ?: return
        if (state.isEnrolled) {
            return
        }
        enrollmentInFlight = true
        lifecycleScope.launch {
            try {
                enrollmentCoordinator.enroll(bootstrap)
            } catch (t: Throwable) {
                Log.w(TAG, "enrollment failed", t)
                showRecovery(
                    stage = "enrollment",
                    message = t.message ?: t.javaClass.simpleName,
                    bootstrapJson = bootstrap.rawJson,
                )
            } finally {
                enrollmentInFlight = false
            }
        }
    }

    private fun maybeApplyManagedApps(state: AgentState) {
        val policyCache = state.policyCache ?: return
        val identity = state.identity ?: return
        val bootstrap = state.bootstrap ?: return
        if (appInstallInFlight) {
            return
        }
        if (lastManagedAppsSnapshotVersion == policyCache.version && state.hasManagedApps) {
            return
        }
        appInstallInFlight = true
        managedAppProgress.value = ManagedAppInstallProgress.VerifyingSnapshot
        lifecycleScope.launch(Dispatchers.IO) {
            try {
                managedAppCoordinator.apply(
                    snapshotJson = policyCache.snapshotJson,
                    deviceSecret = identity.deviceSecret,
                    serverUrl = bootstrap.serverUrl,
                    previousSnapshotJson = state.managedApps?.snapshotJson,
                    onProgress = { progress -> managedAppProgress.value = progress },
                )
                stateStore.saveManagedApps(
                    com.xmdm.launcher.state.ManagedAppsState(
                        snapshotJson = policyCache.snapshotJson,
                        lastAppliedAtEpochMillis = System.currentTimeMillis(),
                    ),
                )
                lastManagedAppsSnapshotVersion = policyCache.version
            } catch (t: Throwable) {
                Log.w(TAG, "managed app install failed", t)
                managedAppProgress.value = ManagedAppInstallProgress.Failed(t.message ?: t.javaClass.simpleName)
                withContext(Dispatchers.Main) {
                    showRecovery(
                        stage = "app-install",
                        message = t.message ?: t.javaClass.simpleName,
                        bootstrapJson = bootstrap.rawJson,
                    )
                }
            } finally {
                withContext(Dispatchers.Main) {
                    appInstallInFlight = false
                    renderUi()
                }
            }
        }
    }

    private suspend fun consumeBootstrapIntent() {
        val rawBootstrapJson = resolveBootstrapJson()
            ?: return
        val provisioningRunId = intent.getStringExtra(EXTRA_PROVISIONING_RUN_ID)
        if (provisioningRunId != null && provisioningRunId == lastProvisioningRunId) {
            return
        }

        try {
            if (intent.getBooleanExtra(EXTRA_RESET_STATE, false)) {
                withContext(Dispatchers.IO) {
                    stateStore.clearProvisioningState()
                }
                lastManagedAppsSnapshotVersion = null
                cachedPrettySnapshotJson = null
                cachedPrettySnapshotText = ""
                managedAppProgress.value = ManagedAppInstallProgress.Idle
                renderManagedAppProgress()
            }
            BootstrapProvisioner(stateStore).persist(rawBootstrapJson)
            if (provisioningRunId != null) {
                lastProvisioningRunId = provisioningRunId
            }
        } catch (t: Throwable) {
            Log.w(TAG, "bootstrap parsing failed", t)
            showRecovery(
                stage = "bootstrap",
                message = t.message ?: t.javaClass.simpleName,
                bootstrapJson = rawBootstrapJson,
            )
        }
    }

    private fun showRecovery(stage: String, message: String, bootstrapJson: String? = null) {
        if (recoveryVisible) {
            return
        }
        recoveryVisible = true
        startActivity(
            RecoveryActivity.intent(
                context = this,
                stage = stage,
                message = message,
                bootstrapJson = bootstrapJson,
            ),
        )
    }

    private fun resolveBootstrapJson(): String? {
        intent.getStringExtra(EXTRA_BOOTSTRAP_JSON)?.let { return it }
        intent.getStringExtra(EXTRA_BOOTSTRAP_JSON_B64)?.let { encoded ->
            return String(Base64.decode(encoded, Base64.DEFAULT), Charsets.UTF_8)
        }
        intent.dataString?.let { data ->
            if (data.startsWith(BOOTSTRAP_DATA_PREFIX)) {
                val encoded = data.removePrefix(BOOTSTRAP_DATA_PREFIX)
                return String(
                    Base64.decode(encoded, Base64.URL_SAFE or Base64.NO_WRAP),
                    Charsets.UTF_8,
                )
            }
        }
        return intent.getStringExtra(android.content.Intent.EXTRA_TEXT)
    }

    companion object {
        const val EXTRA_BOOTSTRAP_JSON = "com.xmdm.launcher.EXTRA_BOOTSTRAP_JSON"
        const val EXTRA_BOOTSTRAP_JSON_B64 = "com.xmdm.launcher.EXTRA_BOOTSTRAP_JSON_B64"
        const val EXTRA_RESET_STATE = "com.xmdm.launcher.EXTRA_RESET_STATE"
        const val EXTRA_PROVISIONING_RUN_ID = "com.xmdm.launcher.EXTRA_PROVISIONING_RUN_ID"
        const val BOOTSTRAP_DATA_PREFIX = "base64url:"
        private const val TAG = "XmdmLauncher"
        private val SAVED_AT_FORMATTER = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss z")

        fun intent(
            context: android.content.Context,
            bootstrapJson: String? = null,
            resetState: Boolean = false,
        ): android.content.Intent {
            return android.content.Intent(context, MainActivity::class.java).apply {
                if (bootstrapJson != null) {
                    putExtra(EXTRA_BOOTSTRAP_JSON, bootstrapJson)
                }
                if (resetState) {
                    putExtra(EXTRA_RESET_STATE, true)
                }
            }
        }
    }
}
