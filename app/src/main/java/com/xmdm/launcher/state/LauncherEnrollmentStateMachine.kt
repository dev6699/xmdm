package com.xmdm.launcher.state

sealed interface EnrollmentFlowState {
    data object Idle : EnrollmentFlowState
    data class BootstrapReady(val rawBootstrapJson: String) : EnrollmentFlowState
    data class Enrolling(val rawBootstrapJson: String) : EnrollmentFlowState
    data object Enrolled : EnrollmentFlowState
    data class Failed(val message: String) : EnrollmentFlowState
}

class LauncherEnrollmentStateMachine {
    private var state: EnrollmentFlowState = EnrollmentFlowState.Idle

    val isEnrollmentInFlight: Boolean
        get() = state is EnrollmentFlowState.Enrolling

    val enrollmentError: String?
        get() = (state as? EnrollmentFlowState.Failed)?.message

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
            is EnrollmentFlowState.Failed -> null
        }
    }

    fun onEnrollmentSucceeded() {
        state = EnrollmentFlowState.Enrolled
    }

    fun onEnrollmentFailed(message: String) {
        state = EnrollmentFlowState.Failed(message)
    }

    fun reset() {
        state = EnrollmentFlowState.Idle
    }
}
