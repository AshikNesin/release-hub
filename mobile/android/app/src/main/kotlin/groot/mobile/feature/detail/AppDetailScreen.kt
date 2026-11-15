package groot.mobile.feature.detail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import groot.kit.settings.SettingsInfoRow
import groot.kit.settings.SettingsNote
import groot.kit.settings.SettingsSection
import groot.kit.settings.SettingsSwitchRow
import groot.kit.theme.S
import groot.mobile.R
import groot.mobile.data.InstalledApps
import groot.mobile.data.catalog.AppStatus
import groot.mobile.data.catalog.CatalogEntry
import groot.mobile.data.releasehub.ReleaseHubRelease
import groot.mobile.install.InstallState
import groot.mobile.ui.AppIcon
import groot.mobile.ui.AppIconLoader
import groot.mobile.ui.SystemActions
import groot.mobile.ui.components.GrootTopBar
import groot.mobile.ui.dateLabel
import groot.mobile.ui.sizeLabel
import groot.mobile.ui.versionLabel

@Composable
fun AppDetailRoute(
    viewModel: AppDetailViewModel,
    installedApps: InstalledApps,
    icons: AppIconLoader,
    selfPackage: String,
    onNavigateUp: () -> Unit,
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val needsPermission by viewModel.needsInstallPermission.collectAsStateWithLifecycle()
    val context = LocalContext.current
    LifecycleResumeEffect(Unit) {
        viewModel.onResume()
        onPauseOrDispose {}
    }
    LaunchedEffect(needsPermission) {
        if (needsPermission) {
            SystemActions.installPermission(context)
            viewModel.installPermissionHandled()
        }
    }
    AppDetailScreen(
        state = state,
        icons = icons,
        selfPackage = selfPackage,
        canOpen = state.entry?.let { it.installed != null && installedApps.launchIntent(it.packageName) != null } == true,
        onNavigateUp = onNavigateUp,
        onInstall = { viewModel.install() },
        onInstallRelease = { viewModel.install(it.toManifest()) },
        onOpen = { state.entry?.let { SystemActions.open(context, installedApps.launchIntent(it.packageName)) } },
        onUninstall = { state.entry?.let { SystemActions.uninstall(context, it.packageName) } },
        onAutoUpdateChange = viewModel::setAutoUpdate,
        onDismissError = viewModel::dismissError,
        onRetryHistory = viewModel::loadHistory,
    )
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AppDetailScreen(
    state: AppDetailUiState,
    icons: AppIconLoader,
    selfPackage: String,
    canOpen: Boolean,
    onNavigateUp: () -> Unit,
    onInstall: () -> Unit,
    onInstallRelease: (ReleaseHubRelease) -> Unit,
    onOpen: () -> Unit,
    onUninstall: () -> Unit,
    onAutoUpdateChange: (Boolean) -> Unit,
    onDismissError: () -> Unit,
    onRetryHistory: () -> Unit,
) {
    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    val entry = state.entry
    Scaffold(
        modifier = Modifier.nestedScroll(scrollBehavior.nestedScrollConnection),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = { GrootTopBar(title = entry?.title.orEmpty(), scrollBehavior = scrollBehavior, onNavigateUp = onNavigateUp) },
    ) { innerPadding ->
        Column(
            Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(rememberScrollState())
                .padding(start = S.lg, end = S.lg, top = S.xs, bottom = S.xxl),
            verticalArrangement = Arrangement.spacedBy(S.xl),
        ) {
            if (entry == null) {
                Text(stringResource(R.string.detail_missing), color = MaterialTheme.colorScheme.onSurfaceVariant)
                return@Column
            }
            Header(entry, icons)
            Actions(entry, state.install, canOpen, entry.packageName == selfPackage, onInstall, onOpen, onUninstall, onDismissError)
            Versions(entry, selfPackage)
            entry.latest?.notes?.takeIf { it.isNotBlank() }?.let { notes ->
                SettingsSection(title = stringResource(R.string.detail_whats_new)) {
                    Text(
                        notes,
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurface,
                        modifier = Modifier.padding(S.lg),
                    )
                }
            }
            if (entry.installed != null) {
                SettingsSection(title = stringResource(R.string.detail_section_updates)) {
                    SettingsSwitchRow(
                        title = stringResource(R.string.detail_auto_update),
                        summary = stringResource(
                            if (state.prefs.autoUpdate) R.string.detail_auto_update_summary else R.string.detail_auto_update_off_globally,
                        ),
                        icon = painterResource(R.drawable.ic_schedule),
                        checked = entry.slug !in state.prefs.excludedSlugs,
                        onCheckedChange = onAutoUpdateChange,
                        enabled = state.prefs.autoUpdate,
                    )
                }
            }
            History(entry, state.history, busy = state.install?.busy == true, onInstallRelease, onRetryHistory)
            SettingsNote(stringResource(R.string.detail_signer_hint))
        }
    }
}

@Composable
private fun Header(entry: CatalogEntry, icons: AppIconLoader) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        AppIcon(entry.packageName, entry.title, icons, size = S.appIcon, key = entry.installed?.versionCode)
        Spacer(Modifier.width(S.lg))
        // The name is already the screen title; the header adds what identifies the app on the device.
        Text(entry.packageName, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun Actions(
    entry: CatalogEntry,
    install: InstallState?,
    canOpen: Boolean,
    isSelf: Boolean,
    onInstall: () -> Unit,
    onOpen: () -> Unit,
    onUninstall: () -> Unit,
    onDismissError: () -> Unit,
) {
    Column(verticalArrangement = Arrangement.spacedBy(S.sm)) {
        when {
            install?.busy == true -> {
                val label = when (install) {
                    InstallState.Queued -> stringResource(R.string.status_queued)
                    is InstallState.Downloading -> install.progress?.let {
                        stringResource(R.string.status_downloading_percent, (it * 100).toInt())
                    } ?: stringResource(R.string.status_downloading)
                    InstallState.WaitingForUser -> stringResource(R.string.status_waiting_for_user)
                    else -> stringResource(R.string.status_installing)
                }
                Text(label, style = MaterialTheme.typography.bodyMedium)
                val progress = (install as? InstallState.Downloading)?.progress
                if (progress != null) {
                    LinearProgressIndicator(progress = { progress }, modifier = Modifier.fillMaxWidth())
                } else if (install != InstallState.WaitingForUser) {
                    LinearProgressIndicator(Modifier.fillMaxWidth())
                }
            }
            else -> {
                val primary = when (entry.status) {
                    AppStatus.UpdateAvailable -> R.string.action_update
                    AppStatus.NotInstalled -> R.string.action_install
                    else -> null
                }
                if (primary != null) {
                    Button(
                        onClick = onInstall,
                        shape = RoundedCornerShape(S.cornerRadius),
                        modifier = Modifier.fillMaxWidth().heightIn(min = S.touchTarget),
                    ) { Text(stringResource(primary)) }
                }
                if (entry.installed != null && !isSelf) {
                    Row(horizontalArrangement = Arrangement.spacedBy(S.sm)) {
                        if (canOpen) {
                            OutlinedButton(onClick = onOpen, shape = RoundedCornerShape(S.cornerRadius), modifier = Modifier.weight(1f)) {
                                Text(stringResource(R.string.action_open))
                            }
                        }
                        OutlinedButton(onClick = onUninstall, shape = RoundedCornerShape(S.cornerRadius), modifier = Modifier.weight(1f)) {
                            Text(stringResource(R.string.action_uninstall))
                        }
                    }
                }
            }
        }
        if (install == InstallState.WaitingForUser) {
            Text(
                stringResource(R.string.status_waiting_for_user),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        (install as? InstallState.Failed)?.let { failed ->
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(failed.message, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
                TextButton(onClick = onDismissError) { Text(stringResource(R.string.action_dismiss)) }
            }
        }
    }
}

@Composable
private fun Versions(entry: CatalogEntry, selfPackage: String) {
    val context = LocalContext.current
    val installed = entry.installed
    val latest = entry.latest
    SettingsSection(title = stringResource(R.string.detail_section_versions)) {
        SettingsInfoRow(
            stringResource(R.string.detail_installed),
            installed?.let { versionLabel(it.versionName, it.versionCode) } ?: stringResource(R.string.detail_not_installed),
            painterResource(R.drawable.ic_info),
        )
        SettingsInfoRow(
            stringResource(R.string.detail_latest),
            latest?.let { versionLabel(it.versionName, it.versionCode) } ?: stringResource(R.string.detail_none),
            painterResource(R.drawable.ic_cloud),
        )
        latest?.let { sizeLabel(context, it.size) }?.let {
            SettingsInfoRow(stringResource(R.string.detail_size), it, painterResource(R.drawable.ic_download))
        }
        latest?.let { dateLabel(it.createdAt) }?.let {
            SettingsInfoRow(stringResource(R.string.detail_published), it, painterResource(R.drawable.ic_schedule))
        }
        if (installed != null) {
            SettingsInfoRow(
                stringResource(R.string.detail_installed_by),
                when (installed.installer) {
                    null -> stringResource(R.string.detail_installed_by_unknown)
                    selfPackage -> stringResource(R.string.detail_installed_by_self)
                    else -> installed.installer
                },
                painterResource(R.drawable.ic_shield),
            )
        }
    }
}

@Composable
private fun History(
    entry: CatalogEntry,
    history: HistoryState,
    busy: Boolean,
    onInstallRelease: (ReleaseHubRelease) -> Unit,
    onRetry: () -> Unit,
) {
    var confirm by remember { mutableStateOf<ReleaseHubRelease?>(null) }
    val context = LocalContext.current
    SettingsSection(title = stringResource(R.string.detail_section_history)) {
        when {
            history.loading -> Row(Modifier.fillMaxWidth().padding(S.lg), horizontalArrangement = Arrangement.Center) {
                CircularProgressIndicator()
            }
            history.error != null -> Row(Modifier.padding(start = S.lg, end = S.sm), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    stringResource(R.string.detail_history_error, history.error),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.weight(1f).padding(vertical = S.md),
                )
                TextButton(onClick = onRetry) { Text(stringResource(R.string.apps_retry)) }
            }
            history.releases.isEmpty() -> Text(
                stringResource(R.string.detail_history_empty),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(S.lg),
            )
            else -> history.releases.forEach { release ->
                val details = listOfNotNull(dateLabel(release.createdAt), sizeLabel(context, release.size)).joinToString(" · ")
                Row(
                    Modifier.fillMaxWidth().heightIn(min = S.touchTarget).padding(start = S.lg, end = S.sm, top = S.sm, bottom = S.sm),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Column(Modifier.weight(1f)) {
                        Text(
                            versionLabel(release.versionName, release.versionCode),
                            style = MaterialTheme.typography.bodyLarge,
                            color = MaterialTheme.colorScheme.onSurface,
                        )
                        if (details.isNotEmpty()) {
                            Text(details, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                    val tag = when (release.versionCode) {
                        entry.installed?.versionCode -> R.string.detail_history_current
                        entry.latest?.versionCode -> R.string.detail_history_latest
                        else -> null
                    }
                    if (tag != null) {
                        Text(
                            stringResource(tag),
                            style = MaterialTheme.typography.labelMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(end = S.md),
                        )
                    } else {
                        TextButton(onClick = { confirm = release }, enabled = !busy) { Text(stringResource(R.string.action_install)) }
                    }
                }
            }
        }
    }
    confirm?.let { release ->
        val older = entry.installed?.let { release.versionCode < it.versionCode } == true
        AlertDialog(
            onDismissRequest = { confirm = null },
            title = { Text(stringResource(R.string.detail_install_version, versionLabel(release.versionName, release.versionCode))) },
            text = if (older) {
                { Text(stringResource(R.string.detail_install_version_body)) }
            } else {
                null
            },
            confirmButton = {
                TextButton(onClick = {
                    confirm = null
                    onInstallRelease(release)
                }) { Text(stringResource(R.string.action_install)) }
            },
            dismissButton = { TextButton(onClick = { confirm = null }) { Text(stringResource(android.R.string.cancel)) } },
        )
    }
    Spacer(Modifier.height(S.xxs))
}
