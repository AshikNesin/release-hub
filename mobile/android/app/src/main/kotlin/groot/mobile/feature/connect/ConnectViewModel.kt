package groot.mobile.feature.connect

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import groot.mobile.data.HubSettings
import groot.mobile.data.hub.HubApi
import groot.mobile.data.hub.HubEndpoint
import groot.mobile.data.hub.HubException
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
    /** A token is already saved for the hub, so the field may stay empty. */
    val hasSavedToken: Boolean = false,
    val checking: Boolean = false,
    val inputError: ConnectError? = null,
    /** The hub's answer when the check failed. */
    val hubError: String? = null,
    val connected: Boolean = false,
)

/** Checks a hub URL + token against the hub before saving them. */
class ConnectViewModel(
    private val api: HubApi,
    private val settings: HubSettings,
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

    fun onUrlChange(url: String) = _state.update { it.copy(url = url, inputError = null, hubError = null) }

    fun onTokenChange(token: String) = _state.update { it.copy(token = token, inputError = null, hubError = null) }

    fun connect() {
        val current = _state.value
        if (current.checking) return
        val url = HubEndpoint.normalizeUrl(current.url)
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
            _state.update { it.copy(url = url, checking = true, hubError = null) }
            val endpoint = HubEndpoint(url, token)
            try {
                api.listApps(endpoint)
                settings.connect(endpoint)
                _state.update { it.copy(checking = false, connected = true, token = "") }
            } catch (e: HubException) {
                _state.update { it.copy(checking = false, hubError = e.message) }
            }
        }
    }

    companion object {
        fun factory(api: HubApi, settings: HubSettings, allowCleartext: Boolean): ViewModelProvider.Factory =
            viewModelFactory { initializer { ConnectViewModel(api, settings, allowCleartext) } }
    }
}
