package groot.mobile.feature.settings

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import groot.kit.buildinfo.BuildInfo
import groot.kit.settings.GrootSettingsScreen
import groot.kit.settings.SettingsAbout
import groot.kit.settings.SettingsSection
import groot.kit.settings.SettingsSwitchRow
import groot.kit.theme.ThemeMode
import groot.mobile.BuildConfig
import groot.mobile.R
import groot.mobile.ui.makerUrl

@Composable
fun SettingsRoute(viewModel: SettingsViewModel, buildInfo: BuildInfo, onNavigateUp: () -> Unit, onOpenAbout: () -> Unit) {
    val themeMode by viewModel.themeMode.collectAsStateWithLifecycle()
    val showHomeIntro by viewModel.showHomeIntro.collectAsStateWithLifecycle()
    SettingsScreen(
        themeMode = themeMode,
        onThemeModeChange = viewModel::setThemeMode,
        showHomeIntro = showHomeIntro,
        onShowHomeIntroChange = viewModel::setShowHomeIntro,
        onResetData = viewModel::resetAll,
        buildInfo = buildInfo,
        onNavigateUp = onNavigateUp,
        onOpenAbout = onOpenAbout,
    )
}

/**
 * The kit's GrootSettingsScreen with this app's own section on top. Add your sections in
 * `appSections`, built from the kit's SettingsSection and settings rows.
 */
@Composable
fun SettingsScreen(
    themeMode: ThemeMode,
    onThemeModeChange: (ThemeMode) -> Unit,
    showHomeIntro: Boolean,
    onShowHomeIntroChange: (Boolean) -> Unit,
    onResetData: () -> Unit,
    buildInfo: BuildInfo,
    onNavigateUp: () -> Unit,
    onOpenAbout: () -> Unit,
) {
    GrootSettingsScreen(
        themeMode = themeMode,
        onThemeModeChange = onThemeModeChange,
        onNavigateUp = onNavigateUp,
        onOpenAbout = onOpenAbout,
        onResetData = onResetData,
        about = SettingsAbout(
            appName = stringResource(R.string.app_name),
            buildInfo = buildInfo,
            feedbackEmail = stringResource(R.string.config_feedback_email),
            rateUrl = "https://play.google.com/store/apps/details?id=${BuildConfig.APPLICATION_ID}",
            makerName = stringResource(R.string.config_maker_name),
            makerUrl = makerUrl(),
        ),
    ) {
        SettingsSection(title = stringResource(R.string.settings_section_home)) {
            SettingsSwitchRow(
                title = stringResource(R.string.settings_home_intro),
                summary = stringResource(R.string.settings_home_intro_summary),
                icon = painterResource(R.drawable.ic_dashboard),
                checked = showHomeIntro,
                onCheckedChange = onShowHomeIntroChange,
            )
        }
    }
}
