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
import com.xmdm.launcher.certificates.AndroidCertificateInstaller
import com.xmdm.launcher.certificates.CertificateInstallCoordinator
import com.xmdm.launcher.certificates.certificateBucketVersion
import com.xmdm.launcher.commands.AndroidDeviceRebooter
import com.xmdm.launcher.commands.DeviceCommandCoordinator
import com.xmdm.launcher.commands.DeviceCommandExecutor
import com.xmdm.launcher.commands.DeviceCommandExecutionResult
import com.xmdm.launcher.commands.HttpDeviceCommandGateway
import com.xmdm.launcher.commands.MqttDeviceCommandConfig
import com.xmdm.launcher.commands.MqttDeviceCommandTransport
import com.xmdm.launcher.files.ManagedFileInstallCoordinator
import com.xmdm.launcher.databinding.ActivityMainBinding
import com.xmdm.launcher.enrollment.EnrollmentCoordinator
import com.xmdm.launcher.enrollment.HttpEnrollmentGateway
import com.xmdm.launcher.deviceinfo.DeviceInfoReporter
import com.xmdm.launcher.kiosk.AndroidKioskModeHost
import com.xmdm.launcher.kiosk.KioskModeController
import com.xmdm.launcher.logs.DeviceLogCoordinator
import com.xmdm.launcher.logs.DeviceLogStore
import com.xmdm.launcher.logs.HttpDeviceLogGateway
import com.xmdm.launcher.packages.AndroidPackageRulesHost
import com.xmdm.launcher.packages.PackageRulesController
import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.state.AgentStateStore
import com.xmdm.launcher.state.CertificatesState
import com.xmdm.launcher.state.LauncherEnrollmentStateMachine
import com.xmdm.launcher.state.ManagedAppsState
import com.xmdm.launcher.state.ManagedFilesState
import com.xmdm.launcher.sync.ConfigSyncEngine
import com.xmdm.launcher.sync.HttpConfigSnapshotFetcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.isActive
import java.io.File
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import kotlin.math.max
import kotlin.math.min
import java.util.UUID

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
    private val managedFileCoordinator by lazy {
        ManagedFileInstallCoordinator(
            downloader = HttpManagedAppDownloader(),
            rootDir = File(applicationContext.filesDir, "managed-files"),
        )
    }
    private val certificateCoordinator by lazy {
        CertificateInstallCoordinator(
            downloader = HttpManagedAppDownloader(),
            installer = AndroidCertificateInstaller(applicationContext),
        )
    }
    private val configSyncEngine by lazy {
        ConfigSyncEngine(
            stateStore = stateStore,
            fetcher = HttpConfigSnapshotFetcher(),
        )
    }
    private val deviceLogCoordinator by lazy {
        DeviceLogCoordinator(
            queue = DeviceLogStore.from(applicationContext),
            gateway = HttpDeviceLogGateway(),
        )
    }
    private val deviceInfoReporter by lazy {
        DeviceInfoReporter(applicationContext)
    }
    private val deviceCommandCoordinator by lazy {
        DeviceCommandCoordinator(
            gateway = HttpDeviceCommandGateway(),
            executor = DeviceCommandExecutor(
                rebootAction = AndroidDeviceRebooter(applicationContext),
                configSyncAction = {
                    requestConfigSyncFromCommand()
                },
            ),
        )
    }
    private val kioskModeController by lazy {
        KioskModeController(AndroidKioskModeHost(this))
    }
    private val packageRulesController by lazy {
        PackageRulesController(AndroidPackageRulesHost(this))
    }
    private val enrollmentStateMachine = LauncherEnrollmentStateMachine()
    private val managedAppProgress = MutableStateFlow<ManagedAppInstallProgress>(ManagedAppInstallProgress.Idle)
    private lateinit var binding: ActivityMainBinding
    private val instanceId = UUID.randomUUID().toString().take(8)
    private var fileInstallInFlight = false
    private var appInstallInFlight = false
    private var certInstallInFlight = false
    private var lastManagedFilesSnapshotVersion: Long? = null
    private var lastCertificatesSnapshotVersion: Long? = null
    private var lastManagedAppsSnapshotVersion: Long? = null
    private var lastEnrollmentAttemptBootstrapJson: String? = null
    private var commandTransportJob: Job? = null
    private var commandTransportTargetKey: String? = null
    private var configSyncJob: Job? = null
    private var configSyncTargetKey: String? = null
    private var deviceLogUploadJob: Job? = null
    private var deviceLogUploadTargetKey: String? = null
    private val prettyJson = GsonBuilder().setPrettyPrinting().create()
    private var cachedPrettySnapshotJson: String? = null
    private var cachedPrettySnapshotText: String = ""
    private var latestState: AgentState = AgentState.empty()

    private data class RuntimeSnapshotConfig(
        val mqttAddress: String?,
        val commandPollIntervalMs: Long,
        val configSyncIntervalMs: Long,
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        Log.w(TAG, "onCreate instance=$instanceId")

        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)

        binding.launcherTitle.text = getString(R.string.launcher_title)
        recordDeviceLog(
            source = "launcher",
            level = "info",
            message = "launcher started",
            payload = mapOf("instanceId" to instanceId),
        )
        lifecycleScope.launch {
            consumeBootstrapIntent()
        }
        lifecycleScope.launch {
            stateStore.state.collectLatest { state ->
                latestState = state
                renderLauncherStatus()
                maybeStartEnrollment(state)
                maybeApplyManagedFiles(state)
                maybeApplyCertificates(state)
                maybeApplyManagedApps(state)
                maybeStartConfigSync(state)
                packageRulesController.apply(state)
                kioskModeController.apply(state)
                maybeStartDeviceLogUpload(state)
                maybeStartCommandTransport(state)
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
        Log.w(TAG, "onNewIntent instance=$instanceId")
        lifecycleScope.launch {
            consumeBootstrapIntent()
        }
    }

    override fun onResume() {
        super.onResume()
        packageRulesController.apply(latestState)
        kioskModeController.apply(latestState)
    }

    private fun renderUi() {
        renderManagedAppProgress()
        renderLauncherStatus()
    }

    private fun renderManagedAppProgress() {
        binding.launcherActivity.text = renderLiveManagedAppStatus(latestState, managedAppProgress.value)
    }

    private fun renderLauncherStatus() {
        binding.launcherStatus.text = renderStatus(latestState, enrollmentStateMachine.isEnrollmentInFlight, enrollmentStateMachine.enrollmentError)
    }

    private fun renderStatus(state: AgentState, enrollmentInFlight: Boolean, enrollmentError: String?): CharSequence {
        val bootstrapLine = if (state.isBootstrapped) {
            "bootstrap: restored"
        } else {
            "bootstrap: empty"
        }
        val enrollmentLine = when {
            state.isEnrolled -> getString(R.string.launcher_enrollment_success)
            enrollmentInFlight -> getString(R.string.launcher_enrollment_in_progress)
            enrollmentError != null -> getString(R.string.launcher_enrollment_failed, enrollmentError)
            else -> getString(R.string.launcher_enrollment_empty)
        }
        val identityLine = when {
            state.isEnrolled -> "device identity: restored"
            enrollmentInFlight -> "device identity: enrolling"
            enrollmentError != null -> "device identity: failed"
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
        val managedFilesLine = if (state.hasManagedFiles) {
            "managed files: restored"
        } else {
            "managed files: empty"
        }
        val certificatesLine = if (state.hasCertificates) {
            "certificates: restored"
        } else {
            "certificates: empty"
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
            append(enrollmentLine)
            append('\n')
            append(identityLine)
            append('\n')
            append(deviceOwnerLine)
            append('\n')
            append(policyLine)
            append('\n')
            append(managedFilesLine)
            append('\n')
            append(certificatesLine)
            append('\n')
            append(managedAppsLine)
        }
    }

    private fun renderLiveManagedAppStatus(state: AgentState, progress: ManagedAppInstallProgress): CharSequence {
        return when (progress) {
            ManagedAppInstallProgress.Idle -> if (state.isEnrolled) {
                getString(R.string.launcher_live_completed)
            } else {
                getString(R.string.launcher_live_idle)
            }
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
            is ManagedAppInstallProgress.Completed -> buildString {
                append(getString(R.string.launcher_live_completed))
                if (progress.installed.isNotEmpty() || progress.uninstalled.isNotEmpty()) {
                    append('\n')
                    append(
                        getString(
                            R.string.launcher_live_completed_details,
                            progress.installed.size,
                            progress.uninstalled.size,
                        ),
                    )
                }
            }
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
        val bootstrap = enrollmentStateMachine.nextEnrollmentBootstrap(state) ?: return
        val bootstrapJson = bootstrap.rawJson?.trim() ?: return
        Log.w(TAG, "maybeStartEnrollment instance=$instanceId bootstrap=${bootstrapJson.hashCode()}")
        if (lastEnrollmentAttemptBootstrapJson == bootstrapJson) {
            Log.w(TAG, "enrollment already attempted instance=$instanceId bootstrap=${bootstrapJson.hashCode()}")
            return
        }
        lastEnrollmentAttemptBootstrapJson = bootstrapJson
        Log.w(TAG, "mark enrollment attempted instance=$instanceId bootstrap=${bootstrapJson.hashCode()}")
        recordDeviceLog(
            source = "enrollment",
            level = "info",
            message = "enrollment attempt started",
            payload = mapOf("bootstrapHash" to bootstrapJson.hashCode()),
        )
        lifecycleScope.launch {
            try {
                val result = enrollmentCoordinator.enroll(bootstrap)
                enrollmentStateMachine.onEnrollmentSucceeded()
                recordDeviceLogSafely(
                    source = "enrollment",
                    level = "info",
                    message = "enrollment succeeded",
                    payload = mapOf("deviceId" to result.identity.deviceId),
                )
                maybeStartConfigSync(bootstrap, result.identity, null)
                requestDeviceInfoUpload()
                try {
                    val cached = configSyncEngine.sync(bootstrap, result.identity)
                    maybeStartConfigSync(bootstrap, result.identity, cached)
                    maybeApplyCertificates(cached.snapshotJson, stateStore.state.first())
                    recordDeviceLogSafely(
                        source = "sync",
                        level = "info",
                        message = "initial config sync succeeded",
                        payload = mapOf(
                            "configRevision" to cached.version,
                            "syncedAtEpochMillis" to cached.lastSyncAtEpochMillis,
                        ),
                    )
                    requestDeviceLogUpload(bootstrap, result.identity)
                    requestDeviceInfoUpload()
                    maybeStartCommandTransport(bootstrap, result.identity, cached)
                } catch (t: Throwable) {
                    if (t is kotlinx.coroutines.CancellationException) {
                        throw t
                    }
                    Log.w(TAG, "initial config sync failed", t)
                    recordDeviceLogSafely(
                        source = "sync",
                        level = "warn",
                        message = "initial config sync failed",
                        payload = mapOf("error" to (t.message ?: t.javaClass.simpleName)),
                    )
                }
            } catch (t: Throwable) {
                Log.w(TAG, "enrollment failed", t)
                recordDeviceLogSafely(
                    source = "enrollment",
                    level = "warn",
                    message = "enrollment failed",
                    payload = mapOf("error" to (t.message ?: t.javaClass.simpleName)),
                )
                enrollmentStateMachine.onEnrollmentFailed(t.message ?: t.javaClass.simpleName)
            }
        }
    }

    private fun maybeStartCommandTransport(state: AgentState) {
        maybeStartCommandTransport(state.bootstrap, state.identity, state.policyCache)
    }

    private fun maybeStartCommandTransport(
        bootstrap: com.xmdm.launcher.state.BootstrapState?,
        identity: com.xmdm.launcher.state.DeviceIdentityState?,
        policyCache: com.xmdm.launcher.state.PolicyCacheState?,
    ) {
        if (bootstrap == null || identity == null) {
            commandTransportJob?.cancel()
            commandTransportJob = null
            commandTransportTargetKey = null
            return
        }
        if (policyCache == null) {
            return
        }
        val runtime = runtimeConfig(policyCache.snapshotJson)
        val mqttAddress = runtime.mqttAddress.orEmpty()
        val targetKey = "${bootstrap.serverUrl}|${identity.deviceId}|${identity.deviceSecret}|${policyCache.version}|$mqttAddress|${runtime.commandPollIntervalMs}"
        if (commandTransportJob?.isActive == true && commandTransportTargetKey == targetKey) {
            return
        }
        commandTransportJob?.cancel()
        commandTransportTargetKey = targetKey
        commandTransportJob = lifecycleScope.launch(Dispatchers.IO) {
            while (isActive) {
                try {
                    val mqtt = mqttAddress.takeIf { it.isNotBlank() }
                    if (mqtt != null) {
                        Log.w(TAG, "command transport connecting via mqtt=$mqtt instance=$instanceId")
                        recordDeviceLogSafely(
                            source = "commands",
                            level = "info",
                            message = "command transport connecting via mqtt",
                            payload = mapOf("mqttAddress" to mqtt),
                        )
                        MqttDeviceCommandTransport(
                            MqttDeviceCommandConfig(
                                address = mqtt,
                                clientId = identity.deviceId,
                                deviceId = identity.deviceId,
                                username = identity.deviceId,
                                password = identity.deviceSecret,
                            ),
                        ).stream { command ->
                            deviceCommandCoordinator.handleIncomingCommand(
                                serverUrl = bootstrap.serverUrl,
                                deviceId = identity.deviceId,
                                deviceSecret = identity.deviceSecret,
                                command = command,
                            )
                        }
                    } else {
                        pollCommands(bootstrap, identity)
                    }
                } catch (t: Throwable) {
                    if (t is kotlinx.coroutines.CancellationException) {
                        throw t
                    }
                    Log.w(TAG, "command transport failed", t)
                    recordDeviceLogSafely(
                        source = "commands",
                        level = "warn",
                        message = "command transport failed",
                        payload = mapOf("error" to (t.message ?: t.javaClass.simpleName)),
                    )
                    if (mqttAddress.isNotBlank()) {
                        try {
                            Log.w(TAG, "command transport falling back to polling instance=$instanceId")
                            recordDeviceLogSafely(
                                source = "commands",
                                level = "info",
                                message = "command transport falling back to polling",
                            )
                            pollCommands(bootstrap, identity)
                        } catch (pollFailure: Throwable) {
                            if (pollFailure is kotlinx.coroutines.CancellationException) {
                                throw pollFailure
                            }
                            Log.w(TAG, "command polling fallback failed", pollFailure)
                            recordDeviceLogSafely(
                                source = "commands",
                                level = "warn",
                                message = "command polling fallback failed",
                                payload = mapOf("error" to (pollFailure.message ?: pollFailure.javaClass.simpleName)),
                            )
                        }
                    }
                }
                delay(runtime.commandPollIntervalMs)
            }
        }
    }

    private fun maybeStartConfigSync(state: AgentState) {
        maybeStartConfigSync(state.bootstrap, state.identity, state.policyCache)
    }

    private fun maybeStartConfigSync(
        bootstrap: com.xmdm.launcher.state.BootstrapState?,
        identity: com.xmdm.launcher.state.DeviceIdentityState?,
        policyCache: com.xmdm.launcher.state.PolicyCacheState?,
    ) {
        bootstrap ?: run {
            configSyncJob?.cancel()
            configSyncJob = null
            configSyncTargetKey = null
            return
        }
        identity ?: run {
            configSyncJob?.cancel()
            configSyncJob = null
            configSyncTargetKey = null
            return
        }
        val runtime = runtimeConfig(policyCache?.snapshotJson)
        val syncIntervalMs = runtime.configSyncIntervalMs
        val targetKey = "${bootstrap.serverUrl}|${bootstrap.secondaryServerUrl}|${identity.deviceId}|${identity.deviceSecret}|${policyCache?.version ?: -1}|$syncIntervalMs"
        if (configSyncJob?.isActive == true && configSyncTargetKey == targetKey) {
            return
        }
        configSyncJob?.cancel()
        configSyncTargetKey = targetKey
        configSyncJob = lifecycleScope.launch(Dispatchers.IO) {
            delay(syncIntervalMs)
            while (isActive) {
                try {
                    val cached = configSyncEngine.sync(bootstrap, identity)
                    Log.w(TAG, "config sync refreshed instance=$instanceId")
                    maybeApplyCertificates(cached.snapshotJson, stateStore.state.first())
                    recordDeviceLogSafely(
                        source = "sync",
                        level = "info",
                        message = "config sync refreshed",
                        payload = mapOf(
                            "configRevision" to cached.version,
                            "syncedAtEpochMillis" to cached.lastSyncAtEpochMillis,
                        ),
                    )
                    requestDeviceLogUpload(bootstrap, identity)
                    requestDeviceInfoUpload()
                } catch (t: Throwable) {
                    if (t is kotlinx.coroutines.CancellationException) {
                        throw t
                    }
                    Log.w(TAG, "config sync failed", t)
                    recordDeviceLogSafely(
                        source = "sync",
                        level = "warn",
                        message = "config sync failed",
                        payload = mapOf("error" to (t.message ?: t.javaClass.simpleName)),
                    )
                }
                delay(syncIntervalMs)
            }
        }
    }

    private suspend fun pollCommands(bootstrap: com.xmdm.launcher.state.BootstrapState, identity: com.xmdm.launcher.state.DeviceIdentityState) {
        val handled = deviceCommandCoordinator.pollAndExecute(
            serverUrl = bootstrap.serverUrl,
            deviceId = identity.deviceId,
            deviceSecret = identity.deviceSecret,
        )
        if (handled.isNotEmpty()) {
            Log.w(TAG, "command poll handled ${handled.size} commands instance=$instanceId")
            recordDeviceLogSafely(
                source = "commands",
                level = "info",
                message = "command poll handled commands",
                payload = mapOf("count" to handled.size),
            )
        }
    }

    private suspend fun requestConfigSyncFromCommand(): DeviceCommandExecutionResult {
        val state = stateStore.state.first()
        val bootstrap = state.bootstrap ?: error("bootstrap state unavailable")
        val identity = state.identity ?: error("device identity unavailable")
        val cached = configSyncEngine.sync(bootstrap, identity)
        maybeApplyCertificates(cached.snapshotJson, state)
        recordDeviceLogSafely(
            source = "commands",
            level = "info",
            message = "config sync requested by command",
            payload = mapOf(
                "configRevision" to cached.version,
                "syncedAtEpochMillis" to cached.lastSyncAtEpochMillis,
            ),
        )
        requestDeviceLogUpload(bootstrap, identity)
        requestDeviceInfoUpload()
        return DeviceCommandExecutionResult(
            status = "acked",
            message = "config refreshed",
            details = mapOf(
                "configRevision" to cached.version,
                "syncedAtEpochMillis" to cached.lastSyncAtEpochMillis,
            ),
        )
    }

    private fun maybeApplyManagedFiles(state: AgentState) {
        val policyCache = state.policyCache ?: return
        val identity = state.identity ?: return
        val bootstrap = state.bootstrap ?: return
        if (fileInstallInFlight) {
            return
        }
        if (state.managedFiles?.version == policyCache.version) {
            lastManagedFilesSnapshotVersion = policyCache.version
            return
        }
        if (lastManagedFilesSnapshotVersion == policyCache.version) {
            return
        }
        fileInstallInFlight = true
        lifecycleScope.launch(Dispatchers.IO) {
            try {
                managedFileCoordinator.apply(
                    snapshotJson = policyCache.snapshotJson,
                    deviceSecret = identity.deviceSecret,
                    serverUrl = bootstrap.serverUrl,
                    previousSnapshotJson = state.managedFiles?.snapshotJson,
                )
                stateStore.saveManagedFiles(
                    ManagedFilesState(
                        snapshotJson = policyCache.snapshotJson,
                        version = policyCache.version,
                        lastAppliedAtEpochMillis = System.currentTimeMillis(),
                    ),
                )
                lastManagedFilesSnapshotVersion = policyCache.version
                recordDeviceLogSafely(
                    source = "files",
                    level = "info",
                    message = "managed files applied",
                    payload = mapOf("version" to policyCache.version),
                )
                requestDeviceLogUpload(bootstrap, identity)
                requestDeviceInfoUpload()
            } catch (t: Throwable) {
                Log.w(TAG, "managed file install failed", t)
                recordDeviceLogSafely(
                    source = "files",
                    level = "warn",
                    message = "managed file install failed",
                    payload = mapOf("error" to (t.message ?: t.javaClass.simpleName)),
                )
            } finally {
                withContext(Dispatchers.Main) {
                    fileInstallInFlight = false
                    renderUi()
                }
            }
        }
    }

    private fun maybeApplyCertificates(state: AgentState) {
        val policyCache = state.policyCache ?: return
        maybeApplyCertificates(policyCache.snapshotJson, state)
    }

    private fun maybeApplyCertificates(snapshotJson: String, state: AgentState) {
        val identity = state.identity ?: return
        val bootstrap = state.bootstrap ?: return
        val snapshotVersion = configSnapshotVersion(snapshotJson)
        val requiresManagedFiles = snapshotHasManagedFiles(snapshotJson)
        if (requiresManagedFiles && state.managedFiles?.version != snapshotVersion) {
            return
        }
        if (requiresManagedFiles && lastManagedFilesSnapshotVersion != null && lastManagedFilesSnapshotVersion != snapshotVersion) {
            return
        }
        val desiredVersion = certificateBucketVersion(snapshotJson)
        if (certInstallInFlight) {
            return
        }
        if (desiredVersion == 0L) {
            if (state.certificates == null && lastCertificatesSnapshotVersion == 0L) {
                return
            }
            certInstallInFlight = true
            lifecycleScope.launch(Dispatchers.IO) {
                try {
                    stateStore.clearCertificates()
                    lastCertificatesSnapshotVersion = 0L
                    recordDeviceLogSafely(
                        source = "certificates",
                        level = "info",
                        message = "certificates cleared",
                        payload = mapOf(
                            "version" to 0L,
                            "installed" to 0,
                        ),
                    )
                    requestDeviceLogUpload(bootstrap, identity)
                    requestDeviceInfoUpload()
                } catch (t: Throwable) {
                    Log.w(TAG, "certificate state update failed", t)
                    recordDeviceLogSafely(
                        source = "certificates",
                        level = "warn",
                        message = "certificate state update failed",
                        payload = mapOf("error" to (t.message ?: t.javaClass.simpleName)),
                    )
                } finally {
                    withContext(Dispatchers.Main) {
                        certInstallInFlight = false
                        renderUi()
                    }
                }
            }
            return
        }
        if (state.certificates?.version == desiredVersion) {
            lastCertificatesSnapshotVersion = desiredVersion
            return
        }
        if (lastCertificatesSnapshotVersion == desiredVersion) {
            return
        }
        certInstallInFlight = true
        lifecycleScope.launch(Dispatchers.IO) {
            try {
                val result = certificateCoordinator.apply(
                    snapshotJson = snapshotJson,
                    deviceSecret = identity.deviceSecret,
                    serverUrl = bootstrap.serverUrl,
                )
                stateStore.saveCertificates(
                    CertificatesState(
                        snapshotJson = snapshotJson,
                        version = desiredVersion,
                        lastAppliedAtEpochMillis = System.currentTimeMillis(),
                    ),
                )
                lastCertificatesSnapshotVersion = desiredVersion
                recordDeviceLogSafely(
                    source = "certificates",
                    level = "info",
                    message = "certificates applied",
                    payload = mapOf(
                        "version" to desiredVersion,
                        "installed" to result.installed.size,
                    ),
                )
                requestDeviceLogUpload(bootstrap, identity)
                requestDeviceInfoUpload()
            } catch (t: Throwable) {
                Log.w(TAG, "certificate install failed", t)
                recordDeviceLogSafely(
                    source = "certificates",
                    level = "warn",
                    message = "certificate install failed",
                    payload = mapOf("error" to (t.message ?: t.javaClass.simpleName)),
                )
            } finally {
                withContext(Dispatchers.Main) {
                    certInstallInFlight = false
                    renderUi()
                }
            }
        }
    }

    private fun maybeApplyManagedApps(state: AgentState) {
        val policyCache = state.policyCache ?: return
        val identity = state.identity ?: return
        val bootstrap = state.bootstrap ?: return
        val requiresManagedFiles = snapshotHasManagedFiles(policyCache.snapshotJson)
        val requiresCertificates = snapshotHasCertificates(policyCache.snapshotJson)
        if (appInstallInFlight) {
            return
        }
        if (requiresManagedFiles && state.managedFiles?.version != policyCache.version) {
            return
        }
        if (requiresManagedFiles && lastManagedFilesSnapshotVersion != null && lastManagedFilesSnapshotVersion != policyCache.version) {
            return
        }
        val desiredCertificateVersion = certificateBucketVersion(policyCache.snapshotJson)
        if (requiresCertificates && state.certificates?.version != desiredCertificateVersion) {
            return
        }
        if (requiresCertificates && lastCertificatesSnapshotVersion != null && lastCertificatesSnapshotVersion != desiredCertificateVersion) {
            return
        }
        if (state.hasManagedApps && state.managedApps?.version == policyCache.version) {
            lastManagedAppsSnapshotVersion = policyCache.version
            return
        }
        if (lastManagedAppsSnapshotVersion == policyCache.version && state.hasManagedApps) {
            return
        }
        appInstallInFlight = true
        managedAppProgress.value = ManagedAppInstallProgress.VerifyingSnapshot
        lifecycleScope.launch(Dispatchers.IO) {
            try {
                val result = managedAppCoordinator.apply(
                    snapshotJson = policyCache.snapshotJson,
                    deviceSecret = identity.deviceSecret,
                    serverUrl = bootstrap.serverUrl,
                    previousSnapshotJson = state.managedApps?.snapshotJson,
                    onProgress = { progress -> managedAppProgress.value = progress },
                )
                stateStore.saveManagedApps(
                    ManagedAppsState(
                        snapshotJson = policyCache.snapshotJson,
                        version = policyCache.version,
                        lastAppliedAtEpochMillis = System.currentTimeMillis(),
                    ),
                )
                lastManagedAppsSnapshotVersion = policyCache.version
                recordDeviceLogSafely(
                    source = "apps",
                    level = "info",
                    message = "managed apps applied",
                    payload = mapOf(
                        "installed" to result.installed.size,
                        "uninstalled" to result.uninstalled.size,
                        "version" to policyCache.version,
                    ),
                )
                requestDeviceLogUpload(bootstrap, identity)
                requestDeviceInfoUpload()
                withContext(Dispatchers.Main) {
                    managedAppProgress.value = ManagedAppInstallProgress.Completed(
                        installed = result.installed,
                        uninstalled = result.uninstalled,
                    )
                }
            } catch (t: Throwable) {
                Log.w(TAG, "managed app install failed", t)
                recordDeviceLogSafely(
                    source = "apps",
                    level = "warn",
                    message = "managed app install failed",
                    payload = mapOf("error" to (t.message ?: t.javaClass.simpleName)),
                )
                managedAppProgress.value = ManagedAppInstallProgress.Failed(t.message ?: t.javaClass.simpleName)
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
        Log.w(TAG, "consumeBootstrapIntent instance=$instanceId bootstrap=${rawBootstrapJson.hashCode()}")

        try {
            val normalizedBootstrap = rawBootstrapJson.trim()
            val shouldReset = normalizedBootstrap.isNotEmpty()
            recordDeviceLogSafely(
                source = "bootstrap",
                level = "info",
                message = "bootstrap intent received",
                payload = mapOf("bootstrapHash" to normalizedBootstrap.hashCode()),
            )

            if (shouldReset) {
                withContext(Dispatchers.IO) {
                    stateStore.clearAllState()
                }
                fileInstallInFlight = false
                appInstallInFlight = false
                lastManagedFilesSnapshotVersion = null
                lastCertificatesSnapshotVersion = null
                lastManagedAppsSnapshotVersion = null
                certInstallInFlight = false
                lastEnrollmentAttemptBootstrapJson = null
                cachedPrettySnapshotJson = null
                cachedPrettySnapshotText = ""
                managedAppProgress.value = ManagedAppInstallProgress.Idle
                renderManagedAppProgress()
            }
            enrollmentStateMachine.reset()
            enrollmentStateMachine.onBootstrapReceived(normalizedBootstrap)
            BootstrapProvisioner(stateStore).persist(rawBootstrapJson)
        } catch (t: Throwable) {
            Log.w(TAG, "bootstrap parsing failed", t)
            recordDeviceLogSafely(
                source = "bootstrap",
                level = "warn",
                message = "bootstrap parsing failed",
                payload = mapOf("error" to (t.message ?: t.javaClass.simpleName)),
            )
        }
    }

    private fun snapshotHasManagedFiles(snapshotJson: String): Boolean {
        return try {
            val files = JsonParser.parseString(snapshotJson)
                .asJsonObject
                .getAsJsonArray("files")
            files != null && files.size() > 0
        } catch (_: Throwable) {
            false
        }
    }

    private fun snapshotHasCertificates(snapshotJson: String): Boolean {
        return try {
            val certificates = JsonParser.parseString(snapshotJson)
                .asJsonObject
                .getAsJsonArray("certificates")
            certificates != null && certificates.size() > 0
        } catch (_: Throwable) {
            false
        }
    }

    private fun configSnapshotVersion(snapshotJson: String): Long {
        return try {
            val snapshot = JsonParser.parseString(snapshotJson).asJsonObject
            snapshot.get("version")?.takeIf { !it.isJsonNull }?.asString?.toLongOrNull() ?: 0L
        } catch (_: Throwable) {
            0L
        }
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

    private fun runtimeConfig(snapshotJson: String?): RuntimeSnapshotConfig {
        if (snapshotJson == null) {
            return RuntimeSnapshotConfig(
                mqttAddress = null,
                commandPollIntervalMs = DEFAULT_COMMAND_POLL_INTERVAL_MS,
                configSyncIntervalMs = DEFAULT_CONFIG_SYNC_INTERVAL_MS,
            )
        }
        val snapshot = runCatching { JsonParser.parseString(snapshotJson).asJsonObject }.getOrNull()
        val runtime = snapshot?.getAsJsonObject("runtime")
        return RuntimeSnapshotConfig(
            mqttAddress = runtime?.get("mqttAddress")?.takeIf { !it.isJsonNull }?.asString?.trim()?.takeIf { it.isNotBlank() },
            commandPollIntervalMs = runtime?.get("commandPollIntervalMs")?.takeIf { !it.isJsonNull }
                ?.asLong
                ?.takeIf { it > 0 }
                ?: DEFAULT_COMMAND_POLL_INTERVAL_MS,
            configSyncIntervalMs = runtime?.get("configSyncIntervalMs")?.takeIf { !it.isJsonNull }
                ?.asLong
                ?.takeIf { it > 0 }
                ?: DEFAULT_CONFIG_SYNC_INTERVAL_MS,
        )
    }

    private fun maybeStartDeviceLogUpload(state: AgentState) {
        maybeStartDeviceLogUpload(state.bootstrap, state.identity)
    }

    private fun maybeStartDeviceLogUpload(
        bootstrap: com.xmdm.launcher.state.BootstrapState?,
        identity: com.xmdm.launcher.state.DeviceIdentityState?,
    ) {
        if (bootstrap == null || identity == null) {
            deviceLogUploadJob?.cancel()
            deviceLogUploadJob = null
            deviceLogUploadTargetKey = null
            return
        }
        val targetKey = "${bootstrap.serverUrl}|${bootstrap.secondaryServerUrl}|${identity.deviceId}|${identity.deviceSecret}"
        if (deviceLogUploadJob?.isActive == true && deviceLogUploadTargetKey == targetKey) {
            return
        }
        deviceLogUploadJob?.cancel()
        deviceLogUploadTargetKey = targetKey
        deviceLogUploadJob = lifecycleScope.launch(Dispatchers.IO) {
            delay(DEVICE_LOG_UPLOAD_INITIAL_DELAY_MS)
            while (isActive) {
                try {
                    val uploaded = deviceLogCoordinator.upload(bootstrap, identity)
                    if (uploaded > 0) {
                        Log.w(TAG, "device logs uploaded count=$uploaded instance=$instanceId")
                    }
                } catch (t: Throwable) {
                    if (t is kotlinx.coroutines.CancellationException) {
                        throw t
                    }
                    Log.w(TAG, "device logs upload failed", t)
                }
                delay(DEVICE_LOG_UPLOAD_INTERVAL_MS)
            }
        }
    }

    private fun recordDeviceLog(
        source: String,
        level: String,
        message: String,
        payload: Map<String, Any?> = emptyMap(),
    ) {
        lifecycleScope.launch(Dispatchers.IO) {
            try {
                recordDeviceLogNow(source, level, message, payload)
            } catch (t: Throwable) {
                if (t is kotlinx.coroutines.CancellationException) {
                    throw t
                }
                Log.w(TAG, "device log record failed", t)
            }
        }
    }

    private suspend fun recordDeviceLogSafely(
        source: String,
        level: String,
        message: String,
        payload: Map<String, Any?> = emptyMap(),
    ) {
        try {
            recordDeviceLogNow(source, level, message, payload)
        } catch (t: Throwable) {
            if (t is kotlinx.coroutines.CancellationException) {
                throw t
            }
            Log.w(TAG, "device log record failed", t)
        }
    }

    private suspend fun recordDeviceLogNow(
        source: String,
        level: String,
        message: String,
        payload: Map<String, Any?> = emptyMap(),
    ) {
        deviceLogCoordinator.record(
            source = source,
            level = level,
            message = message,
            payload = payload.takeIf { it.isNotEmpty() },
        )
    }

    private fun requestDeviceLogUpload(
        bootstrap: com.xmdm.launcher.state.BootstrapState,
        identity: com.xmdm.launcher.state.DeviceIdentityState,
    ) {
        lifecycleScope.launch(Dispatchers.IO) {
            try {
                val uploaded = deviceLogCoordinator.upload(bootstrap, identity)
                if (uploaded > 0) {
                    Log.w(TAG, "device logs uploaded count=$uploaded instance=$instanceId")
                }
            } catch (t: Throwable) {
                if (t is kotlinx.coroutines.CancellationException) {
                    throw t
                }
                Log.w(TAG, "device logs upload failed", t)
            }
        }
    }

    private fun requestDeviceInfoUpload() {
        lifecycleScope.launch(Dispatchers.IO) {
            try {
                val state = stateStore.state.first()
                val bootstrap = state.bootstrap ?: return@launch
                val identity = state.identity ?: return@launch
                deviceInfoReporter.uploadIfNeeded(bootstrap, identity, state)
            } catch (t: Throwable) {
                if (t is kotlinx.coroutines.CancellationException) {
                    throw t
                }
                Log.w(TAG, "device info upload failed", t)
            }
        }
    }

    companion object {
        const val EXTRA_BOOTSTRAP_JSON = "com.xmdm.launcher.EXTRA_BOOTSTRAP_JSON"
        const val EXTRA_BOOTSTRAP_JSON_B64 = "com.xmdm.launcher.EXTRA_BOOTSTRAP_JSON_B64"
        const val BOOTSTRAP_DATA_PREFIX = "base64url:"
        private const val TAG = "XmdmLauncher"
        private const val DEVICE_LOG_UPLOAD_INITIAL_DELAY_MS = 5_000L
        private const val DEVICE_LOG_UPLOAD_INTERVAL_MS = 30_000L
        private const val DEFAULT_COMMAND_POLL_INTERVAL_MS = 30_000L
        private const val DEFAULT_CONFIG_SYNC_INTERVAL_MS = 15 * 60 * 1000L
        private val SAVED_AT_FORMATTER = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss z")

        fun intent(
            context: android.content.Context,
            bootstrapJson: String? = null,
        ): android.content.Intent {
            return android.content.Intent(context, MainActivity::class.java).apply {
                if (bootstrapJson != null) {
                    putExtra(EXTRA_BOOTSTRAP_JSON, bootstrapJson)
                }
            }
        }
    }
}
