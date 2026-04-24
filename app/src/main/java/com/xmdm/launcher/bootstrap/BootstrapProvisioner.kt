package com.xmdm.launcher.bootstrap

import com.xmdm.launcher.state.AgentStateStore
import com.xmdm.launcher.state.BootstrapState

class BootstrapProvisioner(
    private val stateStore: AgentStateStore,
    private val parser: BootstrapPayloadParser = BootstrapPayloadParser(),
) {
    suspend fun persist(rawJson: String): BootstrapState {
        val parsed = parser.parse(rawJson)
        stateStore.saveBootstrap(parsed.bootstrap)
        return parsed.bootstrap
    }
}
