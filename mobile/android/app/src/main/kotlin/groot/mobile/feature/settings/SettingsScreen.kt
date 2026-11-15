package groot.mobile.feature.settings

import android.Manifest
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.core.app.NotificationManagerCompat
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import groot.kit.buildinfo.BuildInfo
import groot.kit.settings.GrootSettingsScreen
import groot.kit.settings.SettingsAbout
import groot.kit.settings.SettingsActionRow
import groot.kit.settings.SettingsChoice
import groot.kit.settings.SettingsChoiceRow
import groot.kit.settings.SettingsInfoRow
import groot.kit.settings.SettingsNavigationRow
import groot.kit.settings.SettingsNote
import groot.kit.settings.SettingsSection
import groot.kit.settings.SettingsSwitchRow
import groot.kit.theme.ThemeMode
import groot.mobile.R
import groot.mobile.data.UpdatePrefs
import groot.mobile.ui.SystemActions
import groot.mobile.ui.dateTimeLabel
import groot.mobile.ui.makerUrl
import java.net.URI

@Composable
fun SettingsRoute(
    viewModel: SettingsViewModel,
    buildInfo: BuildInfo,
    canInstall: () -> Boolean,
    onNavigateUp: () -> Unit,
    onOpenAbout: () -> Unit,
    onOpenReleaseHub: () -> Unit,
) {
    val themeMode by viewModel.themeMode.collectAsStateWithLifecycle()
    val releaseHubUrl by viewModel.releaseHubUrl.collectAsStateWithLifecycle()
    val prefs by viewModel.updatePrefs.collectAsStateWithLifecycle()
    val context = LocalContext.current
    var installAllowed by remember { mutableStateOf(true) }
    var notificationsAllowed by remember { mutableStateOf(true) }
    var askedForNotifications by rememberSaveable { mutableStateOf(false) }
    val recheck = {
        installAllowed = canInstall()
        notificationsAllowed = NotificationManagerCompat.from(context).areNotificationsEnabled()
    }
    LifecycleResumeEffect(Unit) {
        recheck()
        onPauseOrDispose {}
    }
    val notificationPermission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { recheck() }
    SettingsScreen(
        themeMode = themeMode,
        onThemeModeChange = viewModel::setThemeMode,
        releaseHubUrl = releaseHubUrl,
        onOpenReleaseHub = onOpenReleaseHub,
        onDisconnect = viewModel::disconnect,
        prefs = prefs,
        onAutoUpdateChange = viewModel::setAutoUpdate,
        onUnmeteredChange = viewModel::setUnmeteredOnly,
        onIntervalChange = viewModel::setCheckIntervalHours,
        installAllowed = installAllowed,
        notificationsAllowed = notificationsAllowed,
        onOpenInstallPermission = { SystemActions.installPermission(context) },
        onOpenNotifications = {
            // Ask once in-app; after that (or before Android 13) the system page is the only way.
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU && !askedForNotifications) {
                askedForNotifications = true
                notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
            } else {
                SystemActions.notificationSettings(context)
            }
        },
        onResetData = viewModel::resetAll,
        buildInfo = buildInfo,
        onNavigateUp = onNavigateUp,
        onOpenAbout = onOpenAbout,
    )
}

/** The kit's GrootSettingsScreen with Release Hub's sections on top. */
@Composable
fun SettingsScreen(
    themeMode: ThemeMode,
    onThemeModeChange: (ThemeMode) -> Unit,
    releaseHubUrl: String?,
    onOpenReleaseHub: () -> Unit,
    onDisconnect: () -> Unit,
    prefs: UpdatePrefs,
    onAutoUpdateChange: (Boolean) -> Unit,
    onUnmeteredChange: (Boolean) -> Unit,
    onIntervalChange: (Int) -> Unit,
    installAllowed: Boolean,
    notificationsAllowed: Boolean,
    onOpenInstallPermission: () -> Unit,
    onOpenNotifications: () -> Unit,
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
            // No "Rate the app": Release Hub ships from the release hub, not Play. Feedback shows once a real
            // address is set in res/values/config.xml.
            feedbackEmail = stringResource(R.string.config_feedback_email).takeUnless { it.endsWith("@example.com") },
            makerName = stringResource(R.string.config_maker_name),
            makerUrl = makerUrl(),
        ),
    ) {
        SettingsSection(title = stringResource(R.string.settings_section_hub)) {
            SettingsNavigationRow(
                title = stringResource(R.string.settings_hub),
                value = releaseHubUrl?.let(::hostOf) ?: stringResource(R.string.settings_hub_none),
                icon = painterResource(R.drawable.ic_cloud),
                onClick = onOpenReleaseHub,
            )
            if (releaseHubUrl != null) {
                SettingsActionRow(
                    title = stringResource(R.string.settings_disconnect),
                    summary = stringResource(R.string.settings_disconnect_summary),
                    icon = painterResource(R.drawable.ic_logout),
                    destructive = true,
                    onClick = onDisconnect,
                )
            }
        }
        SettingsSection(title = stringResource(R.string.settings_section_updates)) {
            SettingsSwitchRow(
                title = stringResource(R.string.settings_auto_update),
                summary = stringResource(R.string.settings_auto_update_summary),
                icon = painterResource(R.drawable.ic_download),
                checked = prefs.autoUpdate,
                onCheckedChange = onAutoUpdateChange,
            )
            SettingsSwitchRow(
                title = stringResource(R.string.settings_unmetered),
                summary = stringResource(R.string.settings_unmetered_summary),
                icon = painterResource(R.drawable.ic_wifi),
                checked = prefs.unmeteredOnly,
                onCheckedChange = onUnmeteredChange,
            )
            SettingsChoiceRow(
                title = stringResource(R.string.settings_interval),
                icon = painterResource(R.drawable.ic_schedule),
                options = UpdatePrefs.INTERVAL_CHOICES.map { SettingsChoice(it, intervalLabel(it)) },
                selected = prefs.checkIntervalHours,
                onSelect = onIntervalChange,
            )
            SettingsInfoRow(
                title = stringResource(R.string.settings_last_checked),
                value = prefs.lastCheckedAt.takeIf { it > 0 }?.let(::dateTimeLabel) ?: stringResource(R.string.settings_never),
                icon = painterResource(R.drawable.ic_history),
            )
        }
        SettingsSection(title = stringResource(R.string.settings_section_permissions)) {
            SettingsNavigationRow(
                title = stringResource(R.string.settings_install_permission),
                value = stringResource(if (installAllowed) R.string.settings_allowed else R.string.settings_not_allowed),
                icon = painterResource(R.drawable.ic_shield),
                onClick = onOpenInstallPermission,
            )
            SettingsNavigationRow(
                title = stringResource(R.string.settings_notifications),
                value = stringResource(if (notificationsAllowed) R.string.settings_allowed else R.string.settings_not_allowed),
                icon = painterResource(R.drawable.ic_notifications),
                onClick = onOpenNotifications,
            )
        }
        SettingsNote(stringResource(R.string.settings_silent_note))
    }
}

@Composable
private fun intervalLabel(hours: Int): String = when (hours) {
    1 -> stringResource(R.string.settings_interval_hour)
    24 -> stringResource(R.string.settings_interval_day)
    else -> pluralStringResource(R.plurals.settings_interval_hours, hours, hours)
}

private fun hostOf(url: String): String = runCatching { URI(url).host }.getOrNull() ?: url
