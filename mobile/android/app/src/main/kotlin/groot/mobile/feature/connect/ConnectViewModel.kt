package groot.mobile.feature.connect

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import groot.mobile.data.ReleaseHubSettings
import groot.mobile.data.releasehub.ReleaseHubApi
import groot.mobile.data.releasehub.ReleaseHubEndpoint
import groot.mobile.data.releasehub.ReleaseHubException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

enum class ConnectError { InvalidUrl, MissingToken, Cleartext }

data class ConnectUiState(
    val url: String = "",
    val token: String = "",
    /** A token is already saved for the release hub, so the field may stay empty. */
    val hasSavedToken: Boolean = false,
    val checking: Boolean = false,
    val inputError: ConnectError? = null,
    /** The release hub's answer when the check failed. */
    val releaseHubError: String? = null,
    val connected: Boolean = false,
)

/** Checks a release hub URL + token against the release hub before saving them. */
class ConnectViewModel(
    private val api: ReleaseHubApi,
    private val settings: ReleaseHubSettings,
    private val allowCleartext: Boolean,
) : ViewModel() {
    private val _state = MutableStateFlow(ConnectUiState())
    val state: StateFlow<ConnectUiState> = _state.asStateFlow()

    init {
        viewModelScope.launch {
            val url = settings.baseUrl.first().orEmpty()
            val saved = settings.currentEndpoint()
            _state.update { it.copy(url = it.url.ifEmpty { url }, hasSavedToken = saved != null) }
        }
    }

    fun onUrlChange(url: String) = _state.update { it.copy(url = url, inputError = null, releaseHubError = null) }

    fun onTokenChange(token: String) = _state.update { it.copy(token = token, inputError = null, releaseHubError = null) }

    fun connect() {
        val current = _state.value
        if (current.checking) return
        val url = ReleaseHubEndpoint.normalizeUrl(current.url)
            ?: return _state.update { it.copy(inputError = ConnectError.InvalidUrl) }
        if (url.startsWith("http://", ignoreCase = true) && !allowCleartext) {
            return _state.update { it.copy(inputError = ConnectError.Cleartext) }
        }
        viewModelScope.launch {
            val token = current.token.trim().ifEmpty {
                settings.currentEndpoint()?.takeIf { it.baseUrl == url }?.token.orEmpty()
            }
            if (token.isEmpty()) {
                _state.update { it.copy(inputError = ConnectError.MissingToken) }
                return@launch
            }
            _state.update { it.copy(url = url, checking = true, releaseHubError = null) }
            val endpoint = ReleaseHubEndpoint(url, token)
            try {
                api.listApps(endpoint)
                settings.connect(endpoint)
                _state.update { it.copy(checking = false, connected = true, token = "") }
            } catch (e: ReleaseHubException) {
                _state.update { it.copy(checking = false, releaseHubError = e.message) }
            }
        }
    }

    companion object {
        fun factory(api: ReleaseHubApi, settings: ReleaseHubSettings, allowCleartext: Boolean): ViewModelProvider.Factory =
            viewModelFactory { initializer { ConnectViewModel(api, settings, allowCleartext) } }
    }
}
