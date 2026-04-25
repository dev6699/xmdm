package com.xmdm.launcher.state

sealed interface EnrollmentFlowState {
    data object Idle : EnrollmentFlowState
    data class BootstrapReady(val rawBootstrapJson: String) : EnrollmentFlowState
    data class Enrolling(val rawBootstrapJson: String) : EnrollmentFlowState
    data object Enrolled : EnrollmentFlowState
    data class Recovery(val stage: String, val message: String, val bootstrapJson: String?) : EnrollmentFlowState
}

class LauncherEnrollmentStateMachine {
    private var state: EnrollmentFlowState = EnrollmentFlowState.Idle

    val isEnrollmentInFlight: Boolean
        get() = state is EnrollmentFlowState.Enrolling

    fun onBootstrapReceived(rawBootstrapJson: String) {
        val normalized = rawBootstrapJson.trim()
        if (normalized.isEmpty()) {
            return
        }
        state = EnrollmentFlowState.BootstrapReady(normalized)
    }

    fun nextEnrollmentBootstrap(agentState: AgentState): BootstrapState? {
        if (agentState.isEnrolled) {
            state = EnrollmentFlowState.Enrolled
            return null
        }

        val bootstrap = agentState.bootstrap ?: return null
        val rawBootstrapJson = bootstrap.rawJson ?: return null

        return when (state) {
            is EnrollmentFlowState.Idle,
            is EnrollmentFlowState.BootstrapReady -> {
                state = EnrollmentFlowState.Enrolling(rawBootstrapJson)
                bootstrap
            }
            is EnrollmentFlowState.Enrolling -> null
            is EnrollmentFlowState.Enrolled -> null
            is EnrollmentFlowState.Recovery -> null
        }
    }

    fun onEnrollmentSucceeded() {
        state = EnrollmentFlowState.Enrolled
    }

    fun onEnrollmentFailed(stage: String, message: String, bootstrapJson: String?) {
        state = EnrollmentFlowState.Recovery(stage = stage, message = message, bootstrapJson = bootstrapJson)
    }

    fun reset() {
        state = EnrollmentFlowState.Idle
    }
}
