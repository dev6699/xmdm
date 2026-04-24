package com.xmdm.launcher

import android.os.Bundle
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
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
}
