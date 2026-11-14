package groot.mobile.install

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/** Where an install or update of one package is. */
sealed interface InstallState {
    /** Waiting for another install to finish; installs run one at a time. */
    data object Queued : InstallState

    /** [progress] is 0..1, or null while the size is unknown. */
    data class Downloading(val progress: Float?) : InstallState

    data object Installing : InstallState

    /** Android is showing (or has notified) its install confirmation. */
    data object WaitingForUser : InstallState

    data class Failed(val message: String) : InstallState

    /**
     * Work in progress that another install of the same app must not start over. Waiting for the person
     * isn't busy: an unanswered confirmation notification mustn't block installing again.
     */
    val busy: Boolean get() = this == Queued || this is Downloading || this == Installing
}

/** Install state per package name, shared by the screens, the worker and the result receiver. */
class InstallTracker {
    private val _states = MutableStateFlow<Map<String, InstallState>>(emptyMap())
    val states: StateFlow<Map<String, InstallState>> = _states.asStateFlow()

    fun set(packageName: String, state: InstallState) = _states.update { it + (packageName to state) }

    fun clear(packageName: String) = _states.update { it - packageName }

    fun get(packageName: String): InstallState? = _states.value[packageName]
}
