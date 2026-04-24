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
import com.xmdm.launcher.databinding.ActivityMainBinding
import com.xmdm.launcher.enrollment.EnrollmentCoordinator
import com.xmdm.launcher.enrollment.HttpEnrollmentGateway
import com.xmdm.launcher.recovery.RecoveryActivity
import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.state.AgentStateStore
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

class MainActivity : AppCompatActivity() {
    private val stateStore by lazy { AgentStateStore.from(applicationContext) }
    private val enrollmentCoordinator by lazy {
        EnrollmentCoordinator(
            stateStore = stateStore,
            gateway = HttpEnrollmentGateway(),
        )
    }
    private var enrollmentInFlight = false
    private val prettyJson = GsonBuilder().setPrettyPrinting().create()
    private var recoveryVisible = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)

        binding.launcherTitle.text = getString(R.string.launcher_title)
        consumeBootstrapIntent()
        lifecycleScope.launch {
            stateStore.state.collectLatest { state ->
                binding.launcherStatus.text = renderStatus(state, enrollmentInFlight)
                maybeStartEnrollment(state)
            }
        }
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
                append(prettyConfig(policyCache.snapshotJson))
            }
        } else {
            "policy cache: empty\nsaved at: -\nconfig snapshot: empty"
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

    private fun formatSavedAt(instant: Instant): String {
        return SAVED_AT_FORMATTER.format(instant.atZone(ZoneId.systemDefault()))
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

    private fun consumeBootstrapIntent() {
        val rawBootstrapJson = resolveBootstrapJson()
            ?: return

        lifecycleScope.launch {
            try {
                BootstrapProvisioner(stateStore).persist(rawBootstrapJson)
            } catch (t: Throwable) {
                Log.w(TAG, "bootstrap parsing failed", t)
                showRecovery(
                    stage = "bootstrap",
                    message = t.message ?: t.javaClass.simpleName,
                    bootstrapJson = rawBootstrapJson,
                )
            }
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
        const val BOOTSTRAP_DATA_PREFIX = "base64url:"
        private const val TAG = "XmdmLauncher"
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
