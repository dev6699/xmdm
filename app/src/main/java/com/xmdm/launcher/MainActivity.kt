package com.xmdm.launcher

import android.os.Bundle
import android.util.Base64
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import com.xmdm.launcher.bootstrap.BootstrapProvisioner
import com.xmdm.launcher.databinding.ActivityMainBinding
import com.xmdm.launcher.state.AgentState
import com.xmdm.launcher.state.AgentStateStore
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.launch

class MainActivity : AppCompatActivity() {
    private val stateStore by lazy { AgentStateStore.from(applicationContext) }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)

        binding.launcherTitle.text = getString(R.string.launcher_title)
        consumeBootstrapIntent()
        lifecycleScope.launch {
            stateStore.state.collectLatest { state ->
                binding.launcherStatus.text = renderStatus(state)
            }
        }
    }

    private fun renderStatus(state: AgentState): CharSequence {
        val bootstrapLine = if (state.isBootstrapped) {
            "bootstrap: restored"
        } else {
            "bootstrap: empty"
        }
        val identityLine = if (state.isEnrolled) {
            "device identity: restored"
        } else {
            "device identity: empty"
        }
        val policyLine = if (state.hasPolicyCache) {
            "policy cache: restored"
        } else {
            "policy cache: empty"
        }
        return buildString {
            append(getString(R.string.launcher_status))
            append('\n')
            append(bootstrapLine)
            append('\n')
            append(identityLine)
            append('\n')
            append(policyLine)
        }
    }

    private fun consumeBootstrapIntent() {
        val rawBootstrapJson = resolveBootstrapJson()
            ?: return

        lifecycleScope.launch {
            BootstrapProvisioner(stateStore).persist(rawBootstrapJson)
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

    companion object {
        const val EXTRA_BOOTSTRAP_JSON = "com.xmdm.launcher.EXTRA_BOOTSTRAP_JSON"
        const val EXTRA_BOOTSTRAP_JSON_B64 = "com.xmdm.launcher.EXTRA_BOOTSTRAP_JSON_B64"
        const val BOOTSTRAP_DATA_PREFIX = "base64url:"
    }
}
