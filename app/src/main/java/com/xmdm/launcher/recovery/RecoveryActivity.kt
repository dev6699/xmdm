package com.xmdm.launcher.recovery

import android.app.admin.DevicePolicyManager
import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.util.Base64
import android.util.Log
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import com.xmdm.launcher.MainActivity
import com.xmdm.launcher.R
import com.xmdm.launcher.databinding.ActivityRecoveryBinding
import com.xmdm.launcher.state.AgentStateStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withContext
import kotlinx.coroutines.launch

class RecoveryActivity : AppCompatActivity() {
    private val stateStore by lazy { AgentStateStore.from(applicationContext) }
    private var bootstrapJson: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val binding = ActivityRecoveryBinding.inflate(layoutInflater)
        setContentView(binding.root)

        val stage = intent.getStringExtra(EXTRA_STAGE).orEmpty()
        val message = intent.getStringExtra(EXTRA_MESSAGE).orEmpty()
        bootstrapJson = resolveBootstrapJson(intent)

        binding.recoveryTitle.text = getString(R.string.recovery_title)
        binding.recoverySubtitle.text = getString(R.string.recovery_subtitle, stage.ifBlank { "setup" })
        binding.recoveryDetails.text = message.ifBlank { getString(R.string.recovery_no_details) }
        binding.deviceOwnerStatus.text = getString(
            R.string.recovery_device_owner_status,
            if (isDeviceOwnerApp()) getString(R.string.recovery_status_yes) else getString(R.string.recovery_status_no),
        )
        binding.retryButton.isEnabled = bootstrapJson != null

        lifecycleScope.launch {
            if (bootstrapJson == null) {
                bootstrapJson = stateStore.state.first().bootstrap?.rawJson
                binding.retryButton.isEnabled = bootstrapJson != null
            }
        }

        binding.retryButton.setOnClickListener {
            val retryBootstrap = bootstrapJson
            if (retryBootstrap == null) {
                Toast.makeText(this, R.string.recovery_retry_unavailable, Toast.LENGTH_SHORT).show()
                return@setOnClickListener
            }
            startActivity(MainActivity.intent(this, retryBootstrap, resetState = true))
            finish()
        }

        binding.clearStateButton.setOnClickListener {
            binding.clearStateButton.isEnabled = false
            lifecycleScope.launch {
                try {
                    withContext(Dispatchers.IO) {
                        stateStore.clearEnrollmentState()
                    }
                    Toast.makeText(this@RecoveryActivity, R.string.recovery_state_cleared, Toast.LENGTH_SHORT).show()
                } catch (t: Throwable) {
                    Log.w(TAG, "clear local state failed", t)
                    Toast.makeText(
                        this@RecoveryActivity,
                        getString(R.string.recovery_state_clear_failed, t.message ?: t.javaClass.simpleName),
                        Toast.LENGTH_SHORT,
                    ).show()
                } finally {
                    binding.clearStateButton.isEnabled = true
                }
            }
        }
    }

    private fun isDeviceOwnerApp(): Boolean {
        val devicePolicyManager = getSystemService(DevicePolicyManager::class.java) ?: return false
        return devicePolicyManager.isDeviceOwnerApp(packageName)
    }

    companion object {
        private const val EXTRA_STAGE = "com.xmdm.launcher.recovery.EXTRA_STAGE"
        private const val EXTRA_MESSAGE = "com.xmdm.launcher.recovery.EXTRA_MESSAGE"
        private const val EXTRA_BOOTSTRAP_JSON = "com.xmdm.launcher.recovery.EXTRA_BOOTSTRAP_JSON"
        private const val EXTRA_BOOTSTRAP_JSON_B64 = "com.xmdm.launcher.recovery.EXTRA_BOOTSTRAP_JSON_B64"
        private const val TAG = "XmdmRecovery"

        fun intent(
            context: Context,
            stage: String,
            message: String,
            bootstrapJson: String? = null,
        ): Intent {
            return Intent(context, RecoveryActivity::class.java).apply {
                putExtra(EXTRA_STAGE, stage)
                putExtra(EXTRA_MESSAGE, message)
                if (bootstrapJson != null) {
                    putExtra(EXTRA_BOOTSTRAP_JSON, bootstrapJson)
                }
            }
        }

        private fun resolveBootstrapJson(intent: Intent): String? {
            intent.getStringExtra(EXTRA_BOOTSTRAP_JSON)?.let { return it }
            intent.getStringExtra(EXTRA_BOOTSTRAP_JSON_B64)?.let { encoded ->
                return String(Base64.decode(encoded, Base64.DEFAULT), Charsets.UTF_8)
            }
            return null
        }
    }
}
