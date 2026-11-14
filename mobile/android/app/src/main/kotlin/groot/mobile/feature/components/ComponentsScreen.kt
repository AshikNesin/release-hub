package groot.mobile.feature.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import groot.kit.settings.SettingsActionRow
import groot.kit.settings.SettingsChoice
import groot.kit.settings.SettingsChoiceRow
import groot.kit.settings.SettingsInfoRow
import groot.kit.settings.SettingsLinkRow
import groot.kit.settings.SettingsNavigationRow
import groot.kit.settings.SettingsSection
import groot.kit.settings.SettingsSwitchRow
import groot.kit.theme.S
import groot.mobile.R
import groot.mobile.ui.components.GrootTopBar
import kotlinx.coroutines.launch

private const val REPO_URL = "https://github.com/AshikNesin/groot-mobile-app"

/** Every kit building block in one place, so buyers can see the style before writing a screen. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ComponentsScreen(onNavigateUp: () -> Unit, onOpenSettings: () -> Unit) {
    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    val snackbar = remember { SnackbarHostState() }
    val scope = rememberCoroutineScope()
    val uriHandler = LocalUriHandler.current
    val tappedTemplate = stringResource(R.string.components_tapped)
    val tapped: (String) -> Unit = { label ->
        scope.launch {
            snackbar.currentSnackbarData?.dismiss()
            snackbar.showSnackbar(tappedTemplate.format(label))
        }
    }

    var switchOn by rememberSaveable { mutableStateOf(true) }
    var route by rememberSaveable { mutableStateOf<String?>(null) }
    var interval by rememberSaveable { mutableStateOf(Interval.Hourly) }

    Scaffold(
        modifier = Modifier.nestedScroll(scrollBehavior.nestedScrollConnection),
        topBar = {
            GrootTopBar(title = stringResource(R.string.components_title), scrollBehavior = scrollBehavior, onNavigateUp = onNavigateUp)
        },
        snackbarHost = { SnackbarHost(snackbar) },
        containerColor = MaterialTheme.colorScheme.background,
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(rememberScrollState())
                .padding(start = S.lg, end = S.lg, top = S.xs, bottom = S.xxl),
            verticalArrangement = Arrangement.spacedBy(S.xl),
        ) {
            SettingsSection(title = stringResource(R.string.components_section_rows)) {
                SettingsNavigationRow(
                    title = stringResource(R.string.components_nav_row),
                    summary = stringResource(R.string.components_nav_row_summary),
                    icon = painterResource(R.drawable.ic_settings),
                    onClick = onOpenSettings,
                )
                SettingsInfoRow(
                    title = stringResource(R.string.components_info_row),
                    value = stringResource(R.string.components_info_value),
                    icon = painterResource(R.drawable.ic_info),
                )
                SettingsSwitchRow(
                    title = stringResource(R.string.components_switch_row),
                    summary = stringResource(R.string.components_switch_row_summary),
                    icon = painterResource(R.drawable.ic_share),
                    checked = switchOn,
                    onCheckedChange = { switchOn = it },
                )
            }

            SettingsSection(title = stringResource(R.string.components_section_pickers)) {
                SettingsChoiceRow(
                    title = stringResource(R.string.components_route),
                    summary = stringResource(R.string.components_route_summary),
                    icon = painterResource(R.drawable.ic_place),
                    options = buildList {
                        add(SettingsChoice(null, stringResource(R.string.components_route_none)))
                        SamplePeers.forEach { add(SettingsChoice(it.id, it.name, it.address, enabled = it.online)) }
                    },
                    selected = route,
                    onSelect = { route = it },
                )
                SettingsChoiceRow(
                    title = stringResource(R.string.components_interval),
                    icon = painterResource(R.drawable.ic_speed),
                    options = Interval.entries.map { SettingsChoice(it, stringResource(it.label)) },
                    selected = interval,
                    onSelect = { interval = it },
                )
            }

            SettingsSection(title = stringResource(R.string.components_section_actions)) {
                SettingsLinkRow(
                    title = stringResource(R.string.components_link_row),
                    summary = stringResource(R.string.components_link_row_summary),
                    icon = painterResource(R.drawable.ic_code),
                    onClick = { runCatching { uriHandler.openUri(REPO_URL) } },
                )
                val destructive = stringResource(R.string.components_action_row)
                SettingsActionRow(
                    title = destructive,
                    icon = painterResource(R.drawable.ic_delete),
                    destructive = true,
                    onClick = { tapped(destructive) },
                )
            }

            SettingsSection(title = stringResource(R.string.components_section_buttons)) {
                Column(Modifier.padding(S.lg), verticalArrangement = Arrangement.spacedBy(S.sm)) {
                    val primary = stringResource(R.string.components_button_primary)
                    val secondary = stringResource(R.string.components_button_secondary)
                    val text = stringResource(R.string.components_button_text)
                    val buttonModifier = Modifier
                        .fillMaxWidth()
                        .heightIn(min = S.touchTarget + S.sm)
                    Button(onClick = { tapped(primary) }, shape = RoundedCornerShape(S.cornerRadius), modifier = buttonModifier) {
                        Text(primary)
                    }
                    FilledTonalButton(
                        onClick = { tapped(secondary) },
                        shape = RoundedCornerShape(S.cornerRadius),
                        modifier = buttonModifier,
                    ) {
                        Text(secondary)
                    }
                    TextButton(onClick = { tapped(text) }, modifier = Modifier.fillMaxWidth()) { Text(text) }
                }
            }
        }
    }
}

private enum class Interval(val label: Int) {
    Manual(R.string.components_interval_manual),
    QuarterHour(R.string.components_interval_15m),
    Hourly(R.string.components_interval_1h),
    SixHours(R.string.components_interval_6h),
}

private data class Peer(val id: String, val name: String, val address: String, val online: Boolean)

private val SamplePeers = listOf(
    Peer("mac-mini", "home-mac-mini", "100.64.0.10", online = true),
    Peer("codevm", "codevm", "100.64.0.11", online = true),
    Peer("nas", "home-nas", "100.64.0.12", online = true),
    Peer("devbox", "old-devbox", "100.64.0.13", online = false),
)
