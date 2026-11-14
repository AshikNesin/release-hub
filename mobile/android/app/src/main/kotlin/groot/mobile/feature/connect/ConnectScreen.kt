package groot.mobile.feature.connect

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import groot.kit.settings.SettingsNote
import groot.kit.theme.S
import groot.mobile.R
import groot.mobile.ui.components.GrootTopBar

@Composable
fun ConnectRoute(viewModel: ConnectViewModel, onConnected: () -> Unit, onNavigateUp: () -> Unit) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    LaunchedEffect(state.connected) { if (state.connected) onConnected() }
    ConnectScreen(
        state = state,
        onUrlChange = viewModel::onUrlChange,
        onTokenChange = viewModel::onTokenChange,
        onConnect = viewModel::connect,
        onNavigateUp = onNavigateUp,
    )
}

/** Hub URL + API token, checked against the hub before they're saved. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ConnectScreen(
    state: ConnectUiState,
    onUrlChange: (String) -> Unit,
    onTokenChange: (String) -> Unit,
    onConnect: () -> Unit,
    onNavigateUp: () -> Unit,
) {
    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    Scaffold(
        modifier = Modifier.nestedScroll(scrollBehavior.nestedScrollConnection),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = { GrootTopBar(title = stringResource(R.string.connect_title), scrollBehavior = scrollBehavior, onNavigateUp = onNavigateUp) },
    ) { innerPadding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(rememberScrollState())
                .padding(start = S.lg, end = S.lg, top = S.xs, bottom = S.xxl),
            verticalArrangement = Arrangement.spacedBy(S.lg),
        ) {
            Text(
                stringResource(R.string.connect_intro),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            OutlinedTextField(
                value = state.url,
                onValueChange = onUrlChange,
                label = { Text(stringResource(R.string.connect_url)) },
                placeholder = { Text(stringResource(R.string.connect_url_hint)) },
                singleLine = true,
                isError = state.inputError == ConnectError.InvalidUrl || state.inputError == ConnectError.Cleartext,
                supportingText = when (state.inputError) {
                    ConnectError.InvalidUrl -> { { Text(stringResource(R.string.connect_invalid_url)) } }
                    ConnectError.Cleartext -> { { Text(stringResource(R.string.connect_cleartext)) } }
                    else -> null
                },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Next),
                modifier = Modifier.fillMaxWidth(),
            )
            OutlinedTextField(
                value = state.token,
                onValueChange = onTokenChange,
                label = { Text(stringResource(R.string.connect_token)) },
                placeholder = { Text(stringResource(R.string.connect_token_hint)) },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                isError = state.inputError == ConnectError.MissingToken,
                supportingText = when {
                    state.inputError == ConnectError.MissingToken -> { { Text(stringResource(R.string.connect_missing_token)) } }
                    state.hasSavedToken -> { { Text(stringResource(R.string.connect_token_keep)) } }
                    else -> null
                },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = { onConnect() }),
                modifier = Modifier.fillMaxWidth(),
            )
            state.hubError?.let {
                Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
            }
            Button(
                onClick = onConnect,
                enabled = !state.checking,
                shape = RoundedCornerShape(S.cornerRadius),
                modifier = Modifier.fillMaxWidth().heightIn(min = S.touchTarget + S.sm),
            ) {
                Text(stringResource(if (state.checking) R.string.connect_checking else R.string.connect_action))
            }
            SettingsNote(stringResource(R.string.connect_security_note))
        }
    }
}
