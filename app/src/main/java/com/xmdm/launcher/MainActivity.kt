package com.xmdm.launcher

import android.app.admin.DevicePolicyManager
import android.app.KeyguardManager
import android.content.Intent
import android.os.Bundle
import android.os.SystemClock
import android.util.Base64
import android.util.Log
import android.view.MotionEvent
import android.view.View
import android.view.WindowManager
import android.view.KeyEvent
import java.io.EOFException
import android.text.InputType
import android.text.method.DigitsKeyListener
import android.view.inputmethod.EditorInfo
import android.widget.EditText
import android.widget.Toast
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import com.google.gson.GsonBuilder
import com.google.gson.JsonParser
import com.xmdm.launcher.BuildConfig
import com.xmdm.launcher.bootstrap.BootstrapProvisioner
import com.xmdm.launcher.apps.AndroidManagedAppInstaller
import com.xmdm.launcher.apps.HttpManagedAppDownloader
import com.xmdm.launcher.apps.ManagedAppInstallCoordinator
import com.xmdm.launcher.apps.ManagedAppInstallProgress
import com.xmdm.launcher.certificates.AndroidCertificateInstaller
import com.xmdm.launcher.certificates.CertificateInstallCoordinator
import com.xmdm.launcher.certificates.certificateBucketVersion
import com.xmdm.launcher.commands.AndroidDeviceRebooter
import com.xmdm.launcher.commands.AndroidCompanionAppLaunchHost
import com.xmdm.launcher.commands.CompanionAppLaunchCoordinator
import com.xmdm.launcher.commands.DeviceCommandCoordinator
import com.xmdm.launcher.commands.DeviceCommandExecutor
import com.xmdm.launcher.commands.DeviceCommandExecutionResult
import com.xmdm.launcher.commands.HttpDeviceCommandGateway
import com.xmdm.launcher.commands.MqttDeviceCommandConfig
import com.xmdm.launcher.commands.MqttDeviceCommandTransport
import com.xmdm.launcher.commands.DeviceCommandResultStore
import com.xmdm.launcher.files.ManagedFileInstallCoordinator
import com.xmdm.launcher.databinding.ActivityMainBinding
import com.xmdm.launcher.enrollment.EnrollmentCoordinator
import com.xmdm.launcher.enrollment.HttpEnrollmentGateway
import com.xmdm.launcher.deviceinfo.DeviceInfoReporter
import com.xmdm.launcher.kiosk.AndroidKioskModeHost
import com.xmdm.launcher.kiosk.KioskModeController
import com.xmdm.launcher.kiosk.KioskExitGestureTracker
import com.xmdm.launcher.kiosk.kioskExitPasscodeHash
import com.xmdm.launcher.kiosk.kioskExitPasscodeMatches
import com.xmdm.launcher.kiosk.kioskExitPasscodeConfigured
import com.xmdm.launcher.kiosk.isPolicyContentReady
import com.xmdm.launcher.kiosk.kioskModeEnabled
import com.xmdm.launcher.kiosk.kioskPolicyActive
import com.xmdm.launcher.kiosk.kioskKeepScreenOn
import com.xmdm.launcher.kiosk.kioskUnlockOnBoot
import com.xmdm.launcher.logs.DeviceLogCoordinator
import com.xmdm.launcher.logs.DeviceLogStore
import com.xmdm.launcher.logs.HttpDeviceLogGateway
import com.xmdm.launcher.packages.AndroidPackageRulesHost
import com.xmdm.launcher.packages.PackageRulesController
import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.state.AgentStateStore
import com.xmdm.launcher.state.CertificatesState
import com.xmdm.launcher.state.KioskControlState
import com.xmdm.launcher.state.LauncherEnrollmentStateMachine
import com.xmdm.launcher.state.ManagedAppsState
import com.xmdm.launcher.state.ManagedFilesState
import com.xmdm.launcher.sync.ConfigSyncEngine
import com.xmdm.launcher.sync.HttpConfigSnapshotFetcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.coroutines.withContext
import kotlinx.coroutines.isActive
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.io.File
import java.security.MessageDigest
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.UUID
import android.os.UserManager
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
            sessionProvider = {
                val state = stateStore.state.first()
                val bootstrap = state.bootstrap
                val identity = state.identity
                if (bootstrap == null || identity == null) null else bootstrap to identity
            },
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
                kioskExitAction = {
                    requestExitKioskFromCommand()
                },
                companionAppLaunchAction = { command ->
                    requestCompanionAppLaunchFromCommand(command)
                },
            ),
            history = DeviceCommandResultStore.from(applicationContext),
        )
    }
    private val companionAppLaunchCoordinator by lazy {
        CompanionAppLaunchCoordinator(AndroidCompanionAppLaunchHost(this))
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
    private var startedFromBoot = false
    private var launcherRuntimeStarted = false
    private var pendingBootstrapDataString: String? = null
    private val prettyJson = GsonBuilder().setPrettyPrinting().create()
    private var cachedPrettySnapshotJson: String? = null
    private var cachedPrettySnapshotText: String = ""
    private val kioskExitGestureTracker = KioskExitGestureTracker()
    private var kioskAdminMenuDialog: AlertDialog? = null
    private var kioskExitDialog: AlertDialog? = null
    private var kioskPolicySyncDialog: AlertDialog? = null
    private var kioskPolicySyncInFlight = false
    private var latestState: AgentState = AgentState.empty()
    private var pendingDeviceLogFlushSessionKey: String? = null
    private var kioskExitPasscodeRejectedCount = 0
    private val configChangedLogMutex = Mutex()
    private var pendingOpenKioskAdminMenu = false
    private var kioskAdminMenuShouldReapply = false
    private var kioskExitDialogShouldReapply = false

    private data class RuntimeSnapshotConfig(
        val mqttAddress: String?,
        val commandPollIntervalMs: Long,
        val configSyncIntervalMs: Long,
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        Log.w(TAG, "onCreate instance=$instanceId")
        startService(Intent(this, com.xmdm.launcher.kiosk.KioskAdminOverlayService::class.java))

        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)
        binding.launcherVersion.text = getString(
            R.string.launcher_version,
            BuildConfig.VERSION_NAME,
            BuildConfig.VERSION_CODE,
        )
        startedFromBoot = intent.getBooleanExtra(EXTRA_STARTED_FROM_BOOT, false)
        pendingOpenKioskAdminMenu = intent.getBooleanExtra(EXTRA_OPEN_KIOSK_ADMIN_MENU, false)
        kioskAdminMenuShouldReapply = pendingOpenKioskAdminMenu
        captureBootstrapIntentData(intent)

        binding.launcherTitle.text = getString(R.string.launcher_title)
        binding.kioskAdminButton.setOnClickListener {
            requestKioskAdminMenu()
        }
        lifecycleScope.launch {
            managedAppProgress.collectLatest {
                renderManagedAppProgress()
            }
        }
        lifecycleScope.launch {
            if (pendingOpenKioskAdminMenu) {
                pendingOpenKioskAdminMenu = false
                requestKioskAdminMenu()
            }
        }
        lifecycleScope.launch {
            awaitUserUnlockIfNeeded()
            startLauncherRuntime()
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        Log.w(TAG, "onNewIntent instance=$instanceId")
        pendingOpenKioskAdminMenu = intent.getBooleanExtra(EXTRA_OPEN_KIOSK_ADMIN_MENU, false)
        kioskAdminMenuShouldReapply = pendingOpenKioskAdminMenu || kioskAdminMenuShouldReapply
        captureBootstrapIntentData(intent)
        if (launcherRuntimeStarted || isUserUnlocked()) {
            consumeBootstrapIntent()
            if (pendingOpenKioskAdminMenu) {
                pendingOpenKioskAdminMenu = false
                requestKioskAdminMenu()
            }
        }
    }

    override fun onResume() {
        super.onResume()
        packageRulesController.apply(latestState)
        if (kioskAdminMenuShouldReapply) {
            return
        }
        kioskModeController.apply(latestState)
    }

    override fun dispatchTouchEvent(ev: MotionEvent): Boolean {
        handleKioskExitGesture(ev)
        return super.dispatchTouchEvent(ev)
    }

    override fun onDestroy() {
        kioskAdminMenuDialog?.dismiss()
        kioskAdminMenuDialog = null
        kioskExitDialog?.dismiss()
        kioskExitDialog = null
        kioskPolicySyncDialog?.dismiss()
        kioskPolicySyncDialog = null
        super.onDestroy()
    }

    private suspend fun startLauncherRuntime() {
        if (launcherRuntimeStarted) {
            return
        }
        launcherRuntimeStarted = true
        recordDeviceLogSafely(
            source = "launcher",
            level = "info",
            message = "launcher started",
            payload = mapOf(
                "instanceId" to instanceId,
                "appVersionName" to BuildConfig.VERSION_NAME,
                "appVersionCode" to BuildConfig.VERSION_CODE,
                "startedFromBoot" to startedFromBoot,
            ),
        )
        recordLauncherUpgradeIfNeeded()
        if (startedFromBoot) {
            clearBootKioskExitSuppression()
        }
        Log.w(TAG, "launcher runtime preparing bootstrap instance=$instanceId")
        consumeBootstrapIntent()
        lifecycleScope.launch {
            Log.w(TAG, "state collector starting instance=$instanceId")
            stateStore.state.collectLatest { state ->
                latestState = state
                maybeFlushPendingDeviceLogs(state)
                maybeRecordObservedConfigChanged(state)
                Log.w(
                    TAG,
                    "state emission instance=$instanceId enrolled=${state.isEnrolled} " +
                        "policy=${state.policyCache?.version ?: -1} " +
                        "managedApps=${state.managedApps?.version ?: -1} " +
                        "managedFiles=${state.managedFiles?.version ?: -1} " +
                        "certs=${state.certificates?.version ?: -1}",
                )
                renderUi()
                maybeStartEnrollment(state)
                maybeStartConfigSync(state)
                maybeApplyManagedFiles(state)
                maybeApplyCertificates(state)
                maybeApplyManagedApps(state)
                packageRulesController.apply(state)
                if (!kioskAdminMenuShouldReapply) {
                    kioskModeController.apply(state)
                }
                maybeStartCommandTransport(state)
            }
        }
    }

    private suspend fun clearBootKioskExitSuppression() {
        val state = stateStore.state.first()
        val kioskControl = state.kioskControl ?: return
        if (kioskControl.exitSuppressedUntilPolicyVersion == null) {
            return
        }
        stateStore.saveKioskControl(KioskControlState())
        latestState = state.copy(kioskControl = null)
    }

    private fun handleKioskExitGesture(event: MotionEvent) {
        val state = latestState
        if (!state.hasPolicyCache) {
            kioskExitGestureTracker.reset()
            return
        }
        if (kioskExitDialog?.isShowing == true) {
            kioskExitGestureTracker.reset()
            return
        }
        when (event.actionMasked) {
            MotionEvent.ACTION_UP -> {
                if (isTopLeftKioskExitTap(event.x, event.y)) {
                    Log.w(TAG, "kiosk exit hotspot tap instance=$instanceId")
                    if (kioskExitGestureTracker.registerTap(event.eventTime)) {
                        requestKioskAdminMenu()
                    }
                } else {
                    kioskExitGestureTracker.reset()
                }
            }
            MotionEvent.ACTION_CANCEL -> kioskExitGestureTracker.reset()
        }
    }

    private fun isTopLeftKioskExitTap(x: Float, y: Float): Boolean {
        val hotspot = 96f * resources.displayMetrics.density
        return x >= 0f && y >= 0f && x <= hotspot && y <= hotspot
    }

    private fun showKioskExitDialog() {
        val state = latestState
        val policyCache = state.policyCache ?: return
        if (kioskExitPasscodeHash(policyCache.snapshotJson).isNullOrBlank()) {
            return
        }
        if (kioskExitDialog?.isShowing == true) {
            return
        }

        Log.w(TAG, "showing kiosk exit dialog instance=$instanceId")
        val passcodeInput = EditText(this).apply {
            inputType = InputType.TYPE_CLASS_NUMBER or InputType.TYPE_NUMBER_VARIATION_PASSWORD
            keyListener = DigitsKeyListener.getInstance("0123456789")
            imeOptions = EditorInfo.IME_ACTION_DONE
            hint = getString(R.string.kiosk_exit_passcode_hint)
            setSingleLine()
        }
        val dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.kiosk_exit_dialog_title)
            .setMessage(R.string.kiosk_exit_dialog_message)
            .setView(passcodeInput)
            .setNegativeButton(android.R.string.cancel, null)
            .setPositiveButton(R.string.kiosk_exit_dialog_unlock, null)
            .create()

        dialog.setOnShowListener {
            val button = dialog.getButton(AlertDialog.BUTTON_POSITIVE)
            button.setOnClickListener {
                submitKioskExitPasscode(passcodeInput, policyCache.snapshotJson, dialog)
            }
            passcodeInput.setOnEditorActionListener { _, actionId, event ->
                if (actionId == EditorInfo.IME_ACTION_DONE ||
                    (event?.keyCode == KeyEvent.KEYCODE_ENTER && event.action == KeyEvent.ACTION_UP)
                ) {
                    submitKioskExitPasscode(passcodeInput, policyCache.snapshotJson, dialog)
                    true
                } else {
                    false
                }
            }
            passcodeInput.requestFocus()
        }
        dialog.setOnDismissListener {
            kioskExitDialog = null
            kioskExitGestureTracker.reset()
            if (kioskExitDialogShouldReapply) {
                kioskExitDialogShouldReapply = false
                kioskModeController.apply(latestState)
            }
        }
        kioskExitDialog = dialog
        kioskExitDialogShouldReapply = true
        dialog.show()
    }

    private fun submitKioskExitPasscode(
        passcodeInput: EditText,
        snapshotJson: String,
        dialog: AlertDialog,
    ) {
        val candidate = passcodeInput.text?.toString().orEmpty()
        if (candidate.isBlank()) {
            passcodeInput.error = getString(R.string.kiosk_exit_dialog_invalid_code)
            return
        }
        if (kioskExitPasscodeMatches(snapshotJson, candidate)) {
            Log.w(TAG, "kiosk exit passcode accepted instance=$instanceId")
            val rejectedBeforeSuccess = kioskExitPasscodeRejectedCount
            kioskExitPasscodeRejectedCount = 0
            lifecycleScope.launch(Dispatchers.IO) {
                recordDeviceLogSafely(
                    source = "kiosk",
                    level = "warn",
                    message = "kiosk exit passcode accepted",
                    payload = mapOf("rejectedBeforeSuccess" to rejectedBeforeSuccess),
                )
            }
            passcodeInput.error = null
            kioskExitDialogShouldReapply = false
            dialog.dismiss()
            lifecycleScope.launch {
                requestLocalKioskExitFromUser()
            }
        } else {
            Log.w(TAG, "kiosk exit passcode rejected instance=$instanceId")
            kioskExitPasscodeRejectedCount += 1
            if (kioskExitPasscodeRejectedCount == 1 || kioskExitPasscodeRejectedCount % 5 == 0) {
                val rejectedCount = kioskExitPasscodeRejectedCount
                lifecycleScope.launch(Dispatchers.IO) {
                    recordDeviceLogSafely(
                        source = "kiosk",
                        level = "warn",
                        message = "kiosk exit passcode rejected",
                        payload = mapOf("failureCount" to rejectedCount),
                    )
                }
            }
            passcodeInput.setText("")
            passcodeInput.error = getString(R.string.kiosk_exit_dialog_invalid_code)
        }
    }

    private suspend fun requestLocalKioskExitFromUser() {
        val state = stateStore.state.first()
        if (!kioskPolicyActive(state)) {
            return
        }
        Log.w(TAG, "requesting local kiosk exit instance=$instanceId")
        suppressKioskUntilCurrentPolicyVersion(
            state = state,
            source = "kiosk",
            message = "kiosk exit requested locally",
        )
    }

    private suspend fun requestLocalKioskEntryFromUser() {
        val state = stateStore.state.first()
        val policyCache = state.policyCache ?: return
        if (!kioskModeEnabled(policyCache.snapshotJson) ||
            !kioskExitPasscodeConfigured(policyCache.snapshotJson) ||
            !isPolicyContentReady(state, policyCache.version)
        ) {
            withContext(Dispatchers.Main) {
                Toast.makeText(
                    this@MainActivity,
                    getString(R.string.kiosk_admin_passcode_required),
                    Toast.LENGTH_SHORT,
                ).show()
            }
            return
        }
        Log.w(TAG, "requesting local kiosk entry instance=$instanceId")
        val updatedState = state.copy(kioskControl = KioskControlState())
        stateStore.saveKioskControl(updatedState.kioskControl!!)
        withContext(Dispatchers.Main) {
            latestState = updatedState
            kioskModeController.apply(updatedState, forceLaunch = true)
        }
        recordDeviceLogSafely(
            source = "kiosk",
            level = "info",
            message = "kiosk enter requested locally",
            payload = mapOf(
                "policyVersion" to policyCache.version,
            ),
        )
    }

    private fun requestLocalPolicySyncFromUser() {
        if (kioskPolicySyncInFlight) {
            Toast.makeText(
                this@MainActivity,
                getString(R.string.kiosk_admin_sync_in_progress),
                Toast.LENGTH_SHORT,
            ).show()
            return
        }
        kioskPolicySyncInFlight = true
        lifecycleScope.launch(Dispatchers.IO) {
            try {
                recordDeviceLogSafely(
                    source = "config",
                    level = "info",
                    message = "config sync requested",
                    payload = mapOf("trigger" to "kiosk"),
                )
                withContext(Dispatchers.Main) {
                    showKioskPolicySyncDialog()
                }
                delay(250)
                val state = stateStore.state.first()
                val cached = withTimeoutOrNull(15_000) {
                    performPolicySync(
                        state = state,
                        source = "kiosk",
                    )
                }
                val refreshedState = stateStore.state.first()
                withContext(Dispatchers.Main) {
                    dismissKioskPolicySyncDialog()
                    if (cached != null) {
                        latestState = refreshedState
                        kioskModeController.apply(refreshedState)
                        Toast.makeText(
                            this@MainActivity,
                            getString(R.string.kiosk_admin_sync_success),
                            Toast.LENGTH_SHORT,
                        ).show()
                    } else {
                        Toast.makeText(
                            this@MainActivity,
                            getString(R.string.kiosk_admin_sync_timed_out),
                            Toast.LENGTH_SHORT,
                        ).show()
                    }
                }
                if (cached != null) {
                    Log.w(TAG, "local config sync completed instance=$instanceId version=${cached.version}")
                } else {
                    Log.w(TAG, "local config sync timed out instance=$instanceId")
                }
            } catch (t: Throwable) {
                if (t is kotlinx.coroutines.CancellationException) {
                    throw t
                }
                Log.w(TAG, "local config sync failed", t)
                withContext(Dispatchers.Main) {
                    dismissKioskPolicySyncDialog()
                    Toast.makeText(
                        this@MainActivity,
                        getString(R.string.kiosk_admin_sync_failed),
                        Toast.LENGTH_SHORT,
                    ).show()
                }
            } finally {
                withContext(Dispatchers.Main) {
                    dismissKioskPolicySyncDialog()
                }
                kioskPolicySyncInFlight = false
            }
        }
    }

    private fun showKioskPolicySyncDialog() {
        if (kioskPolicySyncDialog?.isShowing == true) {
            return
        }
        val dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.kiosk_admin_sync_title)
            .setMessage(R.string.kiosk_admin_sync_in_progress)
            .setCancelable(false)
            .create()
        dialog.setOnDismissListener {
            kioskPolicySyncDialog = null
        }
        kioskPolicySyncDialog = dialog
        dialog.show()
    }

    private fun dismissKioskPolicySyncDialog() {
        runCatching {
            kioskPolicySyncDialog?.dismiss()
        }
        kioskPolicySyncDialog = null
    }

    private suspend fun performPolicySync(
        state: AgentState,
        source: String,
    ): com.xmdm.launcher.state.PolicyCacheState? {
        return try {
            val bootstrap = state.bootstrap ?: return null
            val identity = state.identity ?: return null
            val previousPolicyVersion = state.policyCache?.version
            val cached = configSyncEngine.sync(bootstrap, identity)
            recordConfigChangedIfNeeded(previousPolicyVersion, cached)
            maybeApplyCertificates(cached.snapshotJson, state)
            requestDeviceInfoUpload()
            cached
        } catch (t: Throwable) {
            if (t is kotlinx.coroutines.CancellationException) {
                throw t
            }
            Log.w(TAG, "$source config sync failed", t)
            recordFailureLog(
                source = "config",
                message = "config sync failed",
                throwable = t,
                payload = mapOf("trigger" to source),
            )
            null
        }
    }

    private suspend fun suppressKioskUntilCurrentPolicyVersion(
        state: AgentState,
        source: String,
        message: String,
        recordLog: Boolean = true,
    ): Long? {
        val policyCache = state.policyCache ?: return null
        val updatedState = state.copy(
            kioskControl = KioskControlState(exitSuppressedUntilPolicyVersion = policyCache.version),
        )
        stateStore.saveKioskControl(updatedState.kioskControl!!)
        withContext(Dispatchers.Main) {
            latestState = updatedState
            kioskModeController.apply(updatedState)
        }
        if (recordLog) {
            recordDeviceLogSafely(
                source = source,
                level = if (source == "commands") "info" else if (message.contains("exit")) "warn" else "info",
                message = message,
                payload = mapOf(
                    "policyVersion" to policyCache.version,
                ),
            )
        }
        return policyCache.version
    }

    private suspend fun flushCommandLifecycleLogsSafely() {
        try {
            deviceLogCoordinator.flushPendingLogsIfSessionAvailable()
        } catch (t: Throwable) {
            if (t is kotlinx.coroutines.CancellationException) {
                throw t
            }
            Log.w(TAG, "command lifecycle log flush failed", t)
        }
    }

    private suspend fun applyCommandSideEffectAfterAckIfNeeded(
        command: com.xmdm.launcher.commands.DeviceCommandRecord,
        transportSource: String,
    ) {
        if (command.type.lowercase() != "exit_kiosk") {
            return
        }
        val state = stateStore.state.first()
        val policyCache = state.policyCache ?: return
        recordDeviceLogSafely(
            source = "commands",
            level = "info",
            message = "kiosk exit requested by command",
            payload = mapOf(
                "transportSource" to transportSource,
                "commandId" to command.id,
                "commandType" to command.type,
                "policyVersion" to policyCache.version,
            ),
        )
        flushCommandLifecycleLogsSafely()
        suppressKioskUntilCurrentPolicyVersion(
            state = state,
            source = "commands",
            message = "kiosk exit requested by command",
            recordLog = false,
        )
    }

    private suspend fun awaitUserUnlockIfNeeded() {
        if (isUserUnlocked()) {
            return
        }
        val policyCache = stateStore.state.first().policyCache
        val keepScreenOn = policyCache?.let { kioskKeepScreenOn(it.snapshotJson) } == true
        val unlockOnBoot = policyCache?.let { kioskUnlockOnBoot(it.snapshotJson) } ?: true
        prepareForUnlockScreen(keepScreenOn)
        Log.w(TAG, "waiting for user unlock instance=$instanceId")
        if (unlockOnBoot) {
            requestDismissKeyguard()
        }
        while (!isUserUnlocked()) {
            delay(250)
        }
    }

    private fun prepareForUnlockScreen(keepScreenOn: Boolean) {
        window.addFlags(
            WindowManager.LayoutParams.FLAG_SHOW_WHEN_LOCKED or
                WindowManager.LayoutParams.FLAG_TURN_SCREEN_ON,
        )
        if (keepScreenOn) {
            window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        } else {
            window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        }
    }

    private fun requestDismissKeyguard() {
        val keyguardManager = getSystemService(KeyguardManager::class.java) ?: return
        if (!keyguardManager.isKeyguardLocked) {
            return
        }
        runCatching {
            keyguardManager.requestDismissKeyguard(
                this,
                object : KeyguardManager.KeyguardDismissCallback() {},
            )
        }.onFailure {
            Log.w(TAG, "failed to request keyguard dismissal", it)
        }
    }

    private fun isUserUnlocked(): Boolean {
        val userManager = getSystemService(UserManager::class.java) ?: return true
        return userManager.isUserUnlocked
    }

    private fun renderUi() {
        renderManagedAppProgress()
        renderLauncherStatus()
        renderKioskExitControl()
    }

    private fun renderManagedAppProgress() {
        binding.launcherActivity.text = renderLiveManagedAppStatus(latestState, managedAppProgress.value)
    }

    private fun renderLauncherStatus() {
        binding.launcherStatus.text = renderStatus(latestState, enrollmentStateMachine.isEnrollmentInFlight, enrollmentStateMachine.enrollmentError)
    }

    private fun renderKioskExitControl() {
        binding.kioskAdminButton.visibility = View.GONE
    }

    private fun requestKioskAdminMenu() {
        lifecycleScope.launch {
            val state = stateStore.state.first()
            latestState = state
            kioskAdminMenuShouldReapply = true
            recordDeviceLogSafely(
                source = "kiosk",
                level = "info",
                message = "kiosk admin menu opened",
                payload = mapOf("policyVersion" to state.policyCache?.version),
            )
            showKioskAdminMenu(state)
        }
    }

    private fun showKioskAdminMenu(state: AgentState) {
        if (kioskAdminMenuDialog?.isShowing == true) {
            return
        }
        kioskExitGestureTracker.reset()
        val kioskAction = if (kioskPolicyActive(state)) {
            getString(R.string.kiosk_admin_menu_exit)
        } else {
            getString(R.string.kiosk_admin_menu_enter)
        }
        val dialog = MaterialAlertDialogBuilder(this)
            .setTitle(R.string.kiosk_admin_menu_title)
            .setItems(
                arrayOf(
                    kioskAction,
                    getString(R.string.kiosk_admin_menu_sync),
                ),
            ) { menuDialog, index ->
                kioskAdminMenuShouldReapply = false
                menuDialog.dismiss()
                when (index) {
                    0 -> {
                        if (kioskPolicyActive(latestState)) {
                            showKioskExitDialog()
                        } else {
                            lifecycleScope.launch { requestLocalKioskEntryFromUser() }
                        }
                    }
                    1 -> requestLocalPolicySyncFromUser()
                }
            }
            .setNegativeButton(android.R.string.cancel, null)
            .create()
        dialog.setOnDismissListener {
            kioskAdminMenuDialog = null
            if (kioskAdminMenuShouldReapply) {
                kioskAdminMenuShouldReapply = false
                kioskModeController.apply(latestState)
            }
        }
        kioskAdminMenuDialog = dialog
        dialog.show()
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

    private suspend fun maybeRecordObservedConfigChanged(state: AgentState) {
        val policyCache = state.policyCache ?: return
        recordConfigChangedIfNeeded(
            previousPolicyVersion = lastConfigChangedLogVersion(),
            cached = policyCache,
        )
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
        lifecycleScope.launch {
            try {
                Log.w(TAG, "enrollment begin instance=$instanceId bootstrap=${bootstrapJson.hashCode()}")
                recordDeviceLogSafely(
                    source = "enrollment",
                    level = "info",
                    message = "enrollment started",
                    payload = mapOf("bootstrapHash" to bootstrapJson.hashCode()),
                )
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
                    Log.w(TAG, "initial config sync begin instance=$instanceId device=${result.identity.deviceId}")
                    val previousPolicyVersion = stateStore.state.first().policyCache?.version
                    val cached = configSyncEngine.sync(bootstrap, result.identity)
                    Log.w(TAG, "initial config sync finished instance=$instanceId version=${cached.version}")
                    recordConfigChangedIfNeeded(previousPolicyVersion, cached)
                    maybeStartConfigSync(bootstrap, result.identity, cached)
                    maybeApplyCertificates(cached.snapshotJson, stateStore.state.first())
                    requestDeviceInfoUpload()
                    maybeStartCommandTransport(bootstrap, result.identity, cached)
                } catch (t: Throwable) {
                    if (t is kotlinx.coroutines.CancellationException) {
                        throw t
                    }
                    Log.w(TAG, "initial config sync failed", t)
                    recordFailureLog(
                        source = "config",
                        message = "config sync failed",
                        throwable = t,
                        payload = mapOf("trigger" to "initial"),
                    )
                }
            } catch (t: Throwable) {
                Log.w(TAG, "enrollment failed", t)
                recordFailureLog(
                    source = "enrollment",
                    message = "enrollment failed",
                    throwable = t,
                    level = "error",
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
                        MqttDeviceCommandTransport(
                            MqttDeviceCommandConfig(
                                address = mqtt,
                                clientId = identity.deviceId,
                                deviceId = identity.deviceId,
                                username = identity.deviceId,
                                password = identity.deviceSecret,
                            ),
                        ).stream { command ->
                            handleDeviceCommand(bootstrap, identity, command, "mqtt")
                        }
                    } else {
                        pollCommands(bootstrap, identity)
                    }
                } catch (t: Throwable) {
                    if (t is kotlinx.coroutines.CancellationException) {
                        throw t
                    }
                    if (t is EOFException) {
                        Log.w(TAG, "command transport disconnected", t)
                    } else {
                        Log.w(TAG, "command transport failed", t)
                    }
                    if (mqttAddress.isNotBlank()) {
                        try {
                            Log.w(TAG, "command transport falling back to polling instance=$instanceId")
                            recordDeviceLogSafely(
                                source = "transport",
                                level = "warn",
                                message = "command transport fallback to polling",
                                payload = mapOf("errorType" to t.javaClass.simpleName),
                            )
                            pollCommands(bootstrap, identity)
                        } catch (pollFailure: Throwable) {
                            if (pollFailure is kotlinx.coroutines.CancellationException) {
                                throw pollFailure
                            }
                            Log.w(TAG, "command polling fallback failed", pollFailure)
                            recordFailureLog(
                                source = "transport",
                                message = "command polling fallback failed",
                                throwable = pollFailure,
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
                    val previousPolicyVersion = stateStore.state.first().policyCache?.version
                    val cached = configSyncEngine.sync(bootstrap, identity)
                    Log.w(TAG, "config sync refreshed instance=$instanceId")
                    recordConfigChangedIfNeeded(previousPolicyVersion, cached)
                    maybeApplyCertificates(cached.snapshotJson, stateStore.state.first())
                    requestDeviceInfoUpload()
                } catch (t: Throwable) {
                    if (t is kotlinx.coroutines.CancellationException) {
                        throw t
                    }
                    Log.w(TAG, "config sync failed", t)
                    recordFailureLog(
                        source = "config",
                        message = "config sync failed",
                        throwable = t,
                        payload = mapOf("trigger" to "periodic"),
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
            transportSource = "polling",
            onCommandReceived = { command ->
                recordDeviceLogSafely(
                    source = "commands",
                    level = "info",
                    message = "command received",
                    payload = mapOf(
                        "transportSource" to "polling",
                        "commandId" to command.id,
                        "commandType" to command.type,
                    ),
                )
            },
            onCommandExecuted = { command, result ->
                recordDeviceLogSafely(
                    source = "commands",
                    level = "info",
                    message = "command executed",
                    payload = mapOf(
                        "transportSource" to "polling",
                        "commandId" to command.id,
                        "status" to result.status,
                    ),
                )
            },
            onCommandAcked = { command, acked ->
                recordDeviceLogSafely(
                    source = "commands",
                    level = "info",
                    message = "command ack sent",
                    payload = mapOf(
                        "transportSource" to "polling",
                        "commandId" to command.id,
                        "status" to acked.status,
                    ),
                )
                flushCommandLifecycleLogsSafely()
                applyCommandSideEffectAfterAckIfNeeded(command, "polling")
            },
            onCommandAckFailed = { command, result, failure ->
                recordDeviceLogSafely(
                    source = "commands",
                    level = "warn",
                    message = "command ack failed",
                    payload = mapOf(
                        "transportSource" to "polling",
                        "commandId" to command.id,
                        "status" to result.status,
                        "errorType" to failure.javaClass.simpleName,
                        "errorMessage" to failure.message?.take(MAX_DEVICE_LOG_ERROR_MESSAGE_CHARS),
                    ),
                )
                flushCommandLifecycleLogsSafely()
            },
        )
        if (handled.isNotEmpty()) {
            Log.w(TAG, "command poll handled ${handled.size} commands instance=$instanceId")
        }
    }

    private suspend fun handleDeviceCommand(
        bootstrap: com.xmdm.launcher.state.BootstrapState,
        identity: com.xmdm.launcher.state.DeviceIdentityState,
        command: com.xmdm.launcher.commands.DeviceCommandRecord,
        transportSource: String,
    ) {
        recordDeviceLogSafely(
            source = "commands",
            level = "info",
            message = "command received",
            payload = mapOf(
                "transportSource" to transportSource,
                "commandId" to command.id,
                "commandType" to command.type,
            ),
        )
        val acked = deviceCommandCoordinator.handleIncomingCommand(
            serverUrl = bootstrap.serverUrl,
            deviceId = identity.deviceId,
            deviceSecret = identity.deviceSecret,
            transportSource = transportSource,
            command = command,
            onCommandExecuted = { executedCommand, result ->
                recordDeviceLogSafely(
                    source = "commands",
                    level = "info",
                    message = "command executed",
                    payload = mapOf(
                        "transportSource" to transportSource,
                        "commandId" to executedCommand.id,
                        "status" to result.status,
                    ),
                )
            },
            onCommandAckFailed = { failedCommand, result, failure ->
                recordDeviceLogSafely(
                    source = "commands",
                    level = "warn",
                    message = "command ack failed",
                    payload = mapOf(
                        "transportSource" to transportSource,
                        "commandId" to failedCommand.id,
                        "status" to result.status,
                        "errorType" to failure.javaClass.simpleName,
                        "errorMessage" to failure.message?.take(MAX_DEVICE_LOG_ERROR_MESSAGE_CHARS),
                    ),
                )
                flushCommandLifecycleLogsSafely()
            },
        )
        recordDeviceLogSafely(
            source = "commands",
            level = "info",
            message = "command ack sent",
            payload = mapOf(
                "transportSource" to transportSource,
                "commandId" to command.id,
                "status" to acked.status,
            ),
        )
        flushCommandLifecycleLogsSafely()
        applyCommandSideEffectAfterAckIfNeeded(command, transportSource)
    }

    private suspend fun requestConfigSyncFromCommand(): DeviceCommandExecutionResult {
        recordDeviceLogSafely(
            source = "config",
            level = "info",
            message = "config sync requested",
            payload = mapOf("trigger" to "commands"),
        )
        val state = stateStore.state.first()
        val cached = performPolicySync(
            state = state,
            source = "commands",
        ) ?: error("bootstrap state unavailable")
        return DeviceCommandExecutionResult(
            status = "acked",
            message = "config refreshed",
            details = mapOf(
                "configRevision" to cached.version,
                "syncedAtEpochMillis" to cached.lastSyncAtEpochMillis,
            ),
        )
    }

    private suspend fun requestExitKioskFromCommand(): DeviceCommandExecutionResult {
        val state = stateStore.state.first()
        val policyCache = state.policyCache ?: error("policy cache unavailable")
        return DeviceCommandExecutionResult(
            status = "acked",
            message = "kiosk exit requested",
            details = mapOf(
                "policyVersion" to policyCache.version,
            ),
        )
    }

    private suspend fun requestCompanionAppLaunchFromCommand(command: com.xmdm.launcher.commands.DeviceCommandRecord): DeviceCommandExecutionResult {
        val state = stateStore.state.first()
        return companionAppLaunchCoordinator.execute(state, command)
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
                val desiredFileCount = countSnapshotArray(policyCache.snapshotJson, "files")
                val previousFileCount = state.managedFiles?.snapshotJson
                    ?.let { countSnapshotArray(it, "files") }
                    ?: 0
                when {
                    desiredFileCount > 0 -> recordDeviceLogSafely(
                        source = "managed_files",
                        level = "info",
                        message = "managed files applied",
                        payload = mapOf(
                            "policyVersion" to policyCache.version,
                            "fileCount" to desiredFileCount,
                            "previousFileCount" to previousFileCount,
                        ),
                    )
                    previousFileCount > 0 -> recordDeviceLogSafely(
                        source = "managed_files",
                        level = "info",
                        message = "managed files cleared",
                        payload = mapOf(
                            "policyVersion" to policyCache.version,
                            "previousFileCount" to previousFileCount,
                        ),
                    )
                }
                requestDeviceInfoUpload()
            } catch (t: Throwable) {
                Log.w(TAG, "managed file install failed", t)
                recordFailureLog(
                    source = "managed_files",
                    message = "managed files apply failed",
                    throwable = t,
                    level = "error",
                    payload = mapOf("policyVersion" to policyCache.version),
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
            val previousCertificates = state.certificates
            if (previousCertificates == null) {
                lastCertificatesSnapshotVersion = 0L
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
                            "policyVersion" to snapshotVersion,
                            "previousCertificateVersion" to previousCertificates.version,
                        ),
                    )
                    requestDeviceInfoUpload()
                } catch (t: Throwable) {
                    Log.w(TAG, "certificate state update failed", t)
                    recordFailureLog(
                        source = "certificates",
                        message = "certificates apply failed",
                        throwable = t,
                        level = "error",
                        payload = mapOf("version" to desiredVersion),
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
                requestDeviceInfoUpload()
            } catch (t: Throwable) {
                Log.w(TAG, "certificate install failed", t)
                recordFailureLog(
                    source = "certificates",
                    message = "certificates apply failed",
                    throwable = t,
                    level = "error",
                    payload = mapOf("version" to desiredVersion),
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
        val desiredAppCount = countSnapshotArray(policyCache.snapshotJson, "apps")
        val previousAppCount = state.managedApps?.snapshotJson
            ?.let { countSnapshotArray(it, "apps") }
            ?: 0
        val hasManagedAppWork = desiredAppCount > 0 || previousAppCount > 0
        val requiresManagedFiles = snapshotHasManagedFiles(policyCache.snapshotJson)
        val requiresCertificates = snapshotHasCertificates(policyCache.snapshotJson)
        if (appInstallInFlight) {
            Log.w(TAG, "managed apps skipped in-flight instance=$instanceId policy=${policyCache.version}")
            return
        }
        if (requiresManagedFiles && state.managedFiles?.version != policyCache.version) {
            Log.w(
                TAG,
                "managed apps waiting for managed files instance=$instanceId policy=${policyCache.version} " +
                    "have=${state.managedFiles?.version ?: -1}",
            )
            return
        }
        if (requiresManagedFiles && lastManagedFilesSnapshotVersion != null && lastManagedFilesSnapshotVersion != policyCache.version) {
            Log.w(
                TAG,
                "managed apps waiting for managed files snapshot refresh instance=$instanceId policy=${policyCache.version} " +
                    "last=${lastManagedFilesSnapshotVersion ?: -1}",
            )
            return
        }
        val desiredCertificateVersion = certificateBucketVersion(policyCache.snapshotJson)
        if (requiresCertificates && state.certificates?.version != desiredCertificateVersion) {
            Log.w(
                TAG,
                "managed apps waiting for certificates instance=$instanceId policy=${policyCache.version} " +
                    "desiredCert=$desiredCertificateVersion have=${state.certificates?.version ?: -1}",
            )
            return
        }
        if (requiresCertificates && lastCertificatesSnapshotVersion != null && lastCertificatesSnapshotVersion != desiredCertificateVersion) {
            Log.w(
                TAG,
                "managed apps waiting for certificates refresh instance=$instanceId policy=${policyCache.version} " +
                    "lastCert=${lastCertificatesSnapshotVersion ?: -1} desiredCert=$desiredCertificateVersion",
            )
            return
        }
        if (state.hasManagedApps && state.managedApps?.version == policyCache.version) {
            lastManagedAppsSnapshotVersion = policyCache.version
            Log.w(TAG, "managed apps already applied instance=$instanceId policy=${policyCache.version}")
            return
        }
        if (lastManagedAppsSnapshotVersion == policyCache.version && state.hasManagedApps) {
            Log.w(TAG, "managed apps already attempted instance=$instanceId policy=${policyCache.version}")
            return
        }
        Log.w(
            TAG,
            "managed apps starting instance=$instanceId policy=${policyCache.version} " +
                "requiresManagedFiles=$requiresManagedFiles requiresCertificates=$requiresCertificates " +
                "hasManagedFiles=${state.managedFiles?.version ?: -1} hasCertificates=${state.certificates?.version ?: -1}",
        )
        appInstallInFlight = true
        managedAppProgress.value = ManagedAppInstallProgress.VerifyingSnapshot
        val applyAttempt = beginManagedAppsApplyAttempt(policyCache.version)
        lifecycleScope.launch(Dispatchers.IO) {
            if (hasManagedAppWork || applyAttempt.resumed) {
                recordDeviceLogSafely(
                    source = "managed_apps",
                    level = "info",
                    message = if (applyAttempt.resumed) "managed apps apply resumed" else "managed apps apply started",
                    payload = mapOf(
                        "policyVersion" to policyCache.version,
                        "attemptId" to applyAttempt.attemptId,
                        "resumed" to applyAttempt.resumed,
                        "appCount" to desiredAppCount,
                        "previousAppCount" to previousAppCount,
                    ),
                )
            }
            try {
                val startedAt = SystemClock.elapsedRealtime()
                val result = managedAppCoordinator.apply(
                    snapshotJson = policyCache.snapshotJson,
                    deviceSecret = identity.deviceSecret,
                    serverUrl = bootstrap.serverUrl,
                    previousSnapshotJson = state.managedApps?.snapshotJson,
                    onProgress = { progress -> managedAppProgress.value = progress },
                )
                Log.w(
                    TAG,
                    "managed apps apply finished instance=$instanceId policy=${policyCache.version} " +
                        "elapsedMs=${SystemClock.elapsedRealtime() - startedAt} installed=${result.installed.size} " +
                        "uninstalled=${result.uninstalled.size}",
                )
                val appliedManagedAppsState = ManagedAppsState(
                    snapshotJson = policyCache.snapshotJson,
                    version = policyCache.version,
                    lastAppliedAtEpochMillis = System.currentTimeMillis(),
                )
                stateStore.saveManagedApps(
                    appliedManagedAppsState,
                )
                lastManagedAppsSnapshotVersion = policyCache.version
                if (result.installed.isNotEmpty() || result.uninstalled.isNotEmpty() || applyAttempt.resumed) {
                    recordDeviceLogSafely(
                        source = "managed_apps",
                        level = "info",
                        message = "managed apps applied",
                        payload = mapOf(
                            "policyVersion" to policyCache.version,
                            "attemptId" to applyAttempt.attemptId,
                            "resumed" to applyAttempt.resumed,
                            "installed" to result.installed.size,
                            "uninstalled" to result.uninstalled.size,
                            "elapsedMs" to (SystemClock.elapsedRealtime() - startedAt),
                        ),
                    )
                }
                requestDeviceInfoUpload()
                finishManagedAppsApplyAttempt(policyCache.version, applyAttempt.attemptId)
                val kioskState = latestState.copy(managedApps = appliedManagedAppsState)
                val precheckReason = kioskLaunchPrecheckReason(kioskState)
                if (precheckReason != null) {
                    withContext(Dispatchers.Main) {
                        Log.w(
                            TAG,
                            "kiosk handoff skipped instance=$instanceId policy=${policyCache.version} reason=$precheckReason",
                        )
                        kioskModeController.apply(kioskState)
                        managedAppProgress.value = ManagedAppInstallProgress.Completed(
                            installed = result.installed,
                            uninstalled = result.uninstalled,
                        )
                    }
                    if (precheckReason != "kiosk_policy_disabled") {
                        recordDeviceLogSafely(
                            source = "kiosk",
                            level = "warn",
                            message = "kiosk app launch skipped",
                            payload = kioskLaunchLogPayload(
                                state = kioskState,
                                policyVersion = policyCache.version,
                                reason = precheckReason,
                            ),
                        )
                    }
                } else {
                    val kioskLaunchSucceeded = withContext(Dispatchers.Main) {
                        Log.w(TAG, "attempting kiosk handoff instance=$instanceId")
                        val launched = kioskModeController.launchConfiguredKioskApp(kioskState)
                        if (!launched) {
                            Log.w(TAG, "kiosk handoff fell back to launcher instance=$instanceId")
                            kioskModeController.apply(
                                kioskState,
                                forceLaunch = true,
                            )
                        }
                        Log.w(TAG, "kiosk handoff finished instance=$instanceId")
                        managedAppProgress.value = ManagedAppInstallProgress.Completed(
                            installed = result.installed,
                            uninstalled = result.uninstalled,
                        )
                        launched
                    }
                    recordDeviceLogSafely(
                        source = "kiosk",
                        level = if (kioskLaunchSucceeded) "info" else "error",
                        message = if (kioskLaunchSucceeded) "kiosk app launched" else "kiosk app launch failed",
                        payload = kioskLaunchLogPayload(
                            state = kioskState,
                            policyVersion = policyCache.version,
                            reason = if (kioskLaunchSucceeded) "launched" else "launch_returned_false",
                        ),
                    )
                }
            } catch (t: Throwable) {
                Log.w(TAG, "managed app install failed", t)
                recordFailureLog(
                    source = "managed_apps",
                    message = "managed app install failed",
                    throwable = t,
                    level = "error",
                    payload = mapOf(
                        "policyVersion" to policyCache.version,
                        "attemptId" to applyAttempt.attemptId,
                    ),
                )
                finishManagedAppsApplyAttempt(policyCache.version, applyAttempt.attemptId)
                managedAppProgress.value = ManagedAppInstallProgress.Failed(t.message ?: t.javaClass.simpleName)
            } finally {
                withContext(Dispatchers.Main) {
                    appInstallInFlight = false
                    renderUi()
                }
            }
        }
    }

    private fun consumeBootstrapIntent() {
        val rawBootstrapJson = resolveBootstrapJson()
            ?: return
        pendingBootstrapDataString = null
        Log.w(TAG, "consumeBootstrapIntent instance=$instanceId bootstrap=${rawBootstrapJson.hashCode()}")

        val normalizedBootstrap = rawBootstrapJson.trim()
        if (normalizedBootstrap.isEmpty()) {
            return
        }
        try {
            val bootstrapEntryStartedAt = SystemClock.elapsedRealtime()
            Log.w(TAG, "bootstrap intake begin instance=$instanceId bootstrap=${rawBootstrapJson.hashCode()}")
            lifecycleScope.launch(Dispatchers.IO) {
                recordDeviceLogSafely(
                    source = "bootstrap",
                    level = "info",
                    message = "bootstrap intent received",
                    payload = mapOf("bootstrapHash" to rawBootstrapJson.hashCode()),
                )
            }
            Log.w(
                TAG,
                "bootstrap intake after log launch instance=$instanceId bootstrap=${rawBootstrapJson.hashCode()} " +
                    "elapsedMs=${SystemClock.elapsedRealtime() - bootstrapEntryStartedAt}",
            )
            val bootstrapStartedAt = SystemClock.elapsedRealtime()
            Log.w(TAG, "bootstrap persistence begin instance=$instanceId bootstrap=${rawBootstrapJson.hashCode()}")
            runBlocking(Dispatchers.IO + NonCancellable) {
                Log.w(
                    TAG,
                    "bootstrap clearAllState begin instance=$instanceId bootstrap=${rawBootstrapJson.hashCode()} " +
                        "elapsedMs=${SystemClock.elapsedRealtime() - bootstrapStartedAt}",
                )
                stateStore.clearAllState()
                Log.w(
                    TAG,
                    "bootstrap state cleared instance=$instanceId bootstrap=${rawBootstrapJson.hashCode()} " +
                        "elapsedMs=${SystemClock.elapsedRealtime() - bootstrapStartedAt}",
                )
                BootstrapProvisioner(stateStore).persist(rawBootstrapJson)
                Log.w(
                    TAG,
                    "bootstrap state persisted instance=$instanceId bootstrap=${rawBootstrapJson.hashCode()} " +
                        "elapsedMs=${SystemClock.elapsedRealtime() - bootstrapStartedAt}",
                )
            }
            Log.w(
                TAG,
                "bootstrap intent applied instance=$instanceId bootstrap=${rawBootstrapJson.hashCode()} " +
                    "elapsedMs=${SystemClock.elapsedRealtime() - bootstrapStartedAt}",
            )
            lifecycleScope.launch(Dispatchers.IO) {
                recordDeviceLogSafely(
                    source = "bootstrap",
                    level = "info",
                    message = "bootstrap persisted",
                    payload = mapOf("bootstrapHash" to rawBootstrapJson.hashCode()),
                )
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
            getSharedPreferences(LAUNCHER_RUNTIME_PREFS, MODE_PRIVATE)
                .edit()
                .remove(KEY_LAST_CONFIG_CHANGED_LOG_VERSION)
                .remove(KEY_MANAGED_APPS_APPLY_POLICY_VERSION)
                .remove(KEY_MANAGED_APPS_APPLY_ATTEMPT_ID)
                .apply()
            managedAppProgress.value = ManagedAppInstallProgress.Idle
            renderManagedAppProgress()
            enrollmentStateMachine.reset()
            enrollmentStateMachine.onBootstrapReceived(normalizedBootstrap)
        } catch (t: Throwable) {
            Log.w(TAG, "bootstrap parsing failed", t)
            lifecycleScope.launch(Dispatchers.IO) {
                recordFailureLog(
                    source = "bootstrap",
                    message = "bootstrap failed",
                    throwable = t,
                    level = "error",
                )
            }
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
        pendingBootstrapDataString?.let { data ->
            if (data.startsWith(BOOTSTRAP_DATA_PREFIX)) {
                val encoded = data.removePrefix(BOOTSTRAP_DATA_PREFIX)
                return String(
                    Base64.decode(encoded, Base64.URL_SAFE or Base64.NO_WRAP),
                    Charsets.UTF_8,
                )
            }
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
        return null
    }

    private fun captureBootstrapIntentData(intent: Intent) {
        val data = intent.dataString ?: return
        if (data.startsWith(BOOTSTRAP_DATA_PREFIX)) {
            pendingBootstrapDataString = data
        }
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

    private suspend fun maybeFlushPendingDeviceLogs(state: AgentState) {
        val bootstrap = state.bootstrap ?: return
        val identity = state.identity ?: return
        val sessionKey = "${bootstrap.serverUrl}|${bootstrap.secondaryServerUrl}|${identity.deviceId}"
        if (pendingDeviceLogFlushSessionKey == sessionKey) {
            return
        }
        pendingDeviceLogFlushSessionKey = sessionKey
        deviceLogCoordinator.flushPendingLogsIfSessionAvailable()
    }

    private suspend fun recordLauncherUpgradeIfNeeded() {
        val prefs = getSharedPreferences(LAUNCHER_RUNTIME_PREFS, MODE_PRIVATE)
        val previousVersionCode = prefs.getInt(KEY_LAST_VERSION_CODE, -1)
        val previousVersionName = prefs.getString(KEY_LAST_VERSION_NAME, null)
        if (previousVersionCode >= 0 && previousVersionCode != BuildConfig.VERSION_CODE) {
            val upgradePayload = mapOf(
                "previousVersionCode" to previousVersionCode,
                "currentVersionCode" to BuildConfig.VERSION_CODE,
                "previousVersionName" to previousVersionName,
                "currentVersionName" to BuildConfig.VERSION_NAME,
            )
            recordDeviceLogSafely(
                source = "launcher",
                level = "info",
                message = "launcher upgraded",
                payload = upgradePayload,
            )
        }
        prefs.edit()
            .putInt(KEY_LAST_VERSION_CODE, BuildConfig.VERSION_CODE)
            .putString(KEY_LAST_VERSION_NAME, BuildConfig.VERSION_NAME)
            .apply()
    }

    private suspend fun recordFailureLog(
        source: String,
        message: String,
        throwable: Throwable,
        level: String = "warn",
        payload: Map<String, Any?> = emptyMap(),
    ) {
        recordDeviceLogSafely(
            source = source,
            level = level,
            message = message,
            payload = payload + mapOf(
                "errorType" to throwable.javaClass.simpleName,
                "errorMessage" to throwable.message?.take(MAX_DEVICE_LOG_ERROR_MESSAGE_CHARS),
            ),
        )
    }

    private fun sha256Hex(value: String): String {
        val bytes = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(Charsets.UTF_8))
        return bytes.joinToString(separator = "") { byte -> "%02x".format(byte) }
    }

    private fun countSnapshotArray(snapshotJson: String, key: String): Int {
        return runCatching {
            JsonParser.parseString(snapshotJson)
                .asJsonObject
                .getAsJsonArray(key)
                ?.size()
                ?: 0
        }.getOrDefault(0)
    }

    private data class ManagedAppApplyAttempt(
        val attemptId: String,
        val resumed: Boolean,
    )

    private fun beginManagedAppsApplyAttempt(policyVersion: Long): ManagedAppApplyAttempt {
        val prefs = getSharedPreferences(LAUNCHER_RUNTIME_PREFS, MODE_PRIVATE)
        val inFlightPolicy = if (prefs.contains(KEY_MANAGED_APPS_APPLY_POLICY_VERSION)) {
            prefs.getLong(KEY_MANAGED_APPS_APPLY_POLICY_VERSION, Long.MIN_VALUE)
        } else {
            null
        }
        val existingAttemptId = prefs.getString(KEY_MANAGED_APPS_APPLY_ATTEMPT_ID, null)
        val resumed = inFlightPolicy == policyVersion && !existingAttemptId.isNullOrBlank()
        val attemptId = existingAttemptId.takeIf { resumed } ?: UUID.randomUUID().toString()
        prefs.edit()
            .putLong(KEY_MANAGED_APPS_APPLY_POLICY_VERSION, policyVersion)
            .putString(KEY_MANAGED_APPS_APPLY_ATTEMPT_ID, attemptId)
            .apply()
        return ManagedAppApplyAttempt(attemptId = attemptId, resumed = resumed)
    }

    private fun finishManagedAppsApplyAttempt(policyVersion: Long, attemptId: String) {
        val prefs = getSharedPreferences(LAUNCHER_RUNTIME_PREFS, MODE_PRIVATE)
        val inFlightPolicy = if (prefs.contains(KEY_MANAGED_APPS_APPLY_POLICY_VERSION)) {
            prefs.getLong(KEY_MANAGED_APPS_APPLY_POLICY_VERSION, Long.MIN_VALUE)
        } else {
            null
        }
        val existingAttemptId = prefs.getString(KEY_MANAGED_APPS_APPLY_ATTEMPT_ID, null)
        if (inFlightPolicy == policyVersion && existingAttemptId == attemptId) {
            prefs.edit()
                .remove(KEY_MANAGED_APPS_APPLY_POLICY_VERSION)
                .remove(KEY_MANAGED_APPS_APPLY_ATTEMPT_ID)
                .apply()
        }
    }

    private fun kioskLaunchLogPayload(
        state: AgentState,
        policyVersion: Long,
        reason: String,
    ): Map<String, Any?> {
        val snapshotJson = state.policyCache?.snapshotJson
        val packageName = snapshotJson?.let { findKioskPackageName(it) }
        return buildMap {
            put("policyVersion", policyVersion)
            put("reason", reason)
            packageName?.let { put("packageName", it) }
        }
    }

    private fun kioskLaunchPrecheckReason(state: AgentState): String? {
        val policyCache = state.policyCache ?: return "policy_cache_missing"
        if (!kioskPolicyEnabled(policyCache.snapshotJson)) {
            return "kiosk_policy_disabled"
        }
        if (!isPolicyContentReady(state, policyCache.version)) {
            return "policy_content_not_ready"
        }
        val packageName = findKioskPackageName(policyCache.snapshotJson)
        if (packageName.isNullOrBlank()) {
            return "kiosk_package_not_configured"
        }
        return try {
            packageManager.getPackageInfo(packageName, 0)
            if (packageManager.getLaunchIntentForPackage(packageName) == null) {
                "launch_intent_missing"
            } else {
                null
            }
        } catch (_: android.content.pm.PackageManager.NameNotFoundException) {
            "package_not_installed"
        } catch (_: Throwable) {
            "package_check_failed"
        }
    }

    private fun kioskPolicyEnabled(snapshotJson: String): Boolean {
        return runCatching {
            JsonParser.parseString(snapshotJson)
                .asJsonObject
                .getAsJsonObject("policy")
                ?.get("kioskMode")
                ?.takeIf { !it.isJsonNull }
                ?.asBoolean
                ?: false
        }.getOrDefault(false)
    }

    private fun findKioskPackageName(snapshotJson: String): String? {
        return runCatching {
            JsonParser.parseString(snapshotJson)
                .asJsonObject
                .getAsJsonObject("policy")
                ?.get("kioskAppPackage")
                ?.takeIf { !it.isJsonNull }
                ?.asString
                ?.trim()
                ?.takeIf { it.isNotBlank() }
        }.getOrNull()
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

    private suspend fun recordConfigChangedIfNeeded(
        previousPolicyVersion: Long?,
        cached: com.xmdm.launcher.state.PolicyCacheState,
    ) {
        val currentRevision = cached.version
        var previousConfigRevisionForPayload: Long? = null

        val shouldLog = configChangedLogMutex.withLock {
            val lastLoggedRevision = lastConfigChangedLogVersion()
            if (lastLoggedRevision == currentRevision) {
                false
            } else {
                previousConfigRevisionForPayload = previousPolicyVersion
                    ?.takeIf { it != currentRevision }
                    ?: lastLoggedRevision?.takeIf { it != currentRevision }
                saveLastConfigChangedLogVersion(currentRevision)
                true
            }
        }
        if (!shouldLog) {
            return
        }

        recordDeviceLogSafely(
            source = "config",
            level = "info",
            message = "config changed",
            payload = mapOf(
                "previousConfigRevision" to previousConfigRevisionForPayload,
                "configRevision" to currentRevision,
                "snapshotHash" to sha256Hex(cached.snapshotJson),
                "appCount" to countSnapshotArray(cached.snapshotJson, "apps"),
                "fileCount" to countSnapshotArray(cached.snapshotJson, "files"),
                "certificateCount" to countSnapshotArray(cached.snapshotJson, "certificates"),
            ),
        )
    }

    private fun lastConfigChangedLogVersion(): Long? {
        val prefs = getSharedPreferences(LAUNCHER_RUNTIME_PREFS, MODE_PRIVATE)
        if (!prefs.contains(KEY_LAST_CONFIG_CHANGED_LOG_VERSION)) {
            return null
        }
        return prefs.getLong(KEY_LAST_CONFIG_CHANGED_LOG_VERSION, 0L)
    }

    private fun saveLastConfigChangedLogVersion(version: Long) {
        getSharedPreferences(LAUNCHER_RUNTIME_PREFS, MODE_PRIVATE)
            .edit()
            .putLong(KEY_LAST_CONFIG_CHANGED_LOG_VERSION, version)
            .apply()
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
                recordFailureLog(
                    source = "device_info",
                    message = "device info upload failed",
                    throwable = t,
                )
            }
        }
    }

    companion object {
        const val EXTRA_STARTED_FROM_BOOT = "com.xmdm.launcher.EXTRA_STARTED_FROM_BOOT"
        const val EXTRA_OPEN_KIOSK_ADMIN_MENU = "com.xmdm.launcher.EXTRA_OPEN_KIOSK_ADMIN_MENU"
        const val BOOTSTRAP_DATA_PREFIX = "base64url:"
        private const val TAG = "XmdmLauncher"
        private const val DEFAULT_COMMAND_POLL_INTERVAL_MS = 30_000L
        private const val DEFAULT_CONFIG_SYNC_INTERVAL_MS = 15 * 60 * 1000L
        private const val LAUNCHER_RUNTIME_PREFS = "launcher_runtime"
        private const val KEY_LAST_VERSION_CODE = "last_version_code"
        private const val KEY_LAST_VERSION_NAME = "last_version_name"
        private const val KEY_LAST_CONFIG_CHANGED_LOG_VERSION = "last_config_changed_log_version"
        private const val KEY_MANAGED_APPS_APPLY_POLICY_VERSION = "managed_apps_apply_policy_version"
        private const val KEY_MANAGED_APPS_APPLY_ATTEMPT_ID = "managed_apps_apply_attempt_id"
        private const val MAX_DEVICE_LOG_ERROR_MESSAGE_CHARS = 200
        private val SAVED_AT_FORMATTER = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss z")

        fun intent(context: android.content.Context): android.content.Intent {
            return android.content.Intent(context, MainActivity::class.java)
        }
    }
}
