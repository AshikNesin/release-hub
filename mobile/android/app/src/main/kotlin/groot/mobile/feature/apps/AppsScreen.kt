package groot.mobile.feature.apps

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.lifecycle.compose.LifecycleResumeEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import groot.kit.settings.SettingsSection
import groot.kit.startup.ReportFullyDrawn
import groot.kit.theme.GrootTheme
import groot.kit.theme.S
import groot.mobile.R
import groot.mobile.data.InstalledApps
import groot.mobile.data.catalog.AppStatus
import groot.mobile.data.catalog.CatalogEntry
import groot.mobile.ui.AppIconLoader
import groot.mobile.ui.SystemActions
import groot.mobile.ui.components.GrootTopBar

const val APPS_CONTENT_TAG = "apps_content"
const val CONNECT_PROMPT_TAG = "connect_prompt"

@Composable
fun AppsRoute(
    viewModel: AppsViewModel,
    installedApps: InstalledApps,
    icons: AppIconLoader,
    onOpenApp: (String) -> Unit,
    onConnect: () -> Unit,
    onOpenSettings: () -> Unit,
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val needsPermission by viewModel.needsInstallPermission.collectAsStateWithLifecycle()
    val context = LocalContext.current
    var canInstall by remember { mutableStateOf(true) }
    LifecycleResumeEffect(Unit) {
        canInstall = viewModel.canInstall()
        viewModel.onResume()
        onPauseOrDispose {}
    }
    LaunchedEffect(needsPermission) {
        if (needsPermission) {
            SystemActions.installPermission(context)
            viewModel.installPermissionHandled()
        }
    }
    AppsScreen(
        state = state,
        canInstall = canInstall,
        icons = icons,
        canOpen = { installedApps.launchIntent(it) != null },
        onRefresh = viewModel::refresh,
        onOpenApp = onOpenApp,
        onAction = { entry, action ->
            when (action) {
                RowAction.Open -> SystemActions.open(context, installedApps.launchIntent(entry.packageName))
                RowAction.Install, RowAction.Update -> viewModel.install(entry)
            }
        },
        onUpdateAll = viewModel::updateAll,
        onAllowInstalls = { SystemActions.installPermission(context) },
        onConnect = onConnect,
        onOpenSettings = onOpenSettings,
    )
}

/** Home: every hub app grouped by what it needs, with install and update actions. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AppsScreen(
    state: AppsUiState,
    canInstall: Boolean,
    icons: AppIconLoader,
    canOpen: (String) -> Boolean,
    onRefresh: () -> Unit,
    onOpenApp: (String) -> Unit,
    onAction: (CatalogEntry, RowAction) -> Unit,
    onUpdateAll: () -> Unit,
    onAllowInstalls: () -> Unit,
    onConnect: () -> Unit,
    onOpenSettings: () -> Unit,
) {
    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    Scaffold(
        modifier = Modifier.nestedScroll(scrollBehavior.nestedScrollConnection),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            GrootTopBar(title = stringResource(R.string.app_name), scrollBehavior = scrollBehavior) {
                if (state.connected == true) {
                    IconButton(onClick = onRefresh, enabled = !state.catalog.refreshing) {
                        Icon(painterResource(R.drawable.ic_refresh), contentDescription = stringResource(R.string.refresh))
                    }
                }
                IconButton(onClick = onOpenSettings) {
                    Icon(painterResource(R.drawable.ic_settings), contentDescription = stringResource(R.string.settings_title))
                }
            }
        },
    ) { innerPadding ->
        Box(
            Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .testTag(APPS_CONTENT_TAG),
        ) {
            when (state.connected) {
                null -> Unit
                false -> ConnectPrompt(onConnect)
                true -> PullToRefreshBox(isRefreshing = state.catalog.refreshing, onRefresh = onRefresh) {
                    AppList(state, canInstall, icons, canOpen, onRefresh, onOpenApp, onAction, onUpdateAll, onAllowInstalls)
                }
            }
        }
    }
    ReportFullyDrawn()
}

@Composable
private fun AppList(
    state: AppsUiState,
    canInstall: Boolean,
    icons: AppIconLoader,
    canOpen: (String) -> Boolean,
    onRefresh: () -> Unit,
    onOpenApp: (String) -> Unit,
    onAction: (CatalogEntry, RowAction) -> Unit,
    onUpdateAll: () -> Unit,
    onAllowInstalls: () -> Unit,
) {
    val catalog = state.catalog
    val groups = catalog.entries.groupBy { it.status }
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(start = S.lg, end = S.lg, top = S.xs, bottom = S.xxl),
        verticalArrangement = Arrangement.spacedBy(S.xl),
    ) {
        if (!canInstall && catalog.entries.isNotEmpty()) {
            item(key = "permission") {
                Banner(
                    title = stringResource(R.string.apps_permission_title),
                    body = stringResource(R.string.apps_permission_body),
                    action = stringResource(R.string.apps_permission_action),
                    onAction = onAllowInstalls,
                )
            }
        }
        catalog.error?.let { error ->
            item(key = "error") {
                Banner(title = null, body = error, action = stringResource(R.string.apps_retry), onAction = onRefresh, isError = true)
            }
        }
        if (!catalog.loaded && catalog.entries.isEmpty()) {
            if (catalog.error == null) {
                item(key = "loading") {
                    Box(Modifier.fillMaxWidth().padding(top = S.xxl), contentAlignment = Alignment.Center) {
                        CircularProgressIndicator()
                    }
                }
            }
            return@LazyColumn
        }
        if (catalog.entries.isEmpty()) {
            item(key = "empty") {
                Text(
                    stringResource(R.string.apps_empty),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth().padding(top = S.xxl, start = S.lg, end = S.lg),
                )
            }
            return@LazyColumn
        }
        val sections = listOf(
            AppStatus.UpdateAvailable to R.string.apps_section_updates,
            AppStatus.NotInstalled to R.string.apps_section_not_installed,
            null to R.string.apps_section_installed,
            AppStatus.NoRelease to R.string.apps_section_no_release,
        )
        for ((status, title) in sections) {
            val entries = if (status == null) {
                groups[AppStatus.UpToDate].orEmpty() + groups[AppStatus.AheadOfHub].orEmpty()
            } else {
                groups[status].orEmpty()
            }
            if (entries.isEmpty()) continue
            item(key = title) {
                Column {
                    if (status == AppStatus.UpdateAvailable && entries.size > 1) {
                        UpdateAllRow(onUpdateAll, enabled = entries.none { state.installs[it.packageName]?.busy == true })
                    }
                    SettingsSection(title = stringResource(title)) {
                        entries.forEach { entry ->
                            AppRow(
                                entry = entry,
                                install = state.installs[entry.packageName],
                                icons = icons,
                                canOpen = entry.installed != null && canOpen(entry.packageName),
                                onClick = { onOpenApp(entry.slug) },
                                onAction = { onAction(entry, it) },
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun UpdateAllRow(onUpdateAll: () -> Unit, enabled: Boolean) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
        TextButton(onClick = onUpdateAll, enabled = enabled) { Text(stringResource(R.string.apps_update_all)) }
    }
}

@Composable
private fun Banner(title: String?, body: String, action: String, onAction: () -> Unit, isError: Boolean = false) {
    val colors = MaterialTheme.colorScheme
    Surface(color = GrootTheme.colors.cardSurface, shape = RoundedCornerShape(S.cornerRadius), modifier = Modifier.fillMaxWidth()) {
        Row(Modifier.padding(start = S.lg, end = S.sm, top = S.md, bottom = S.md), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                if (title != null) {
                    Text(title, style = MaterialTheme.typography.titleSmall, color = colors.onSurface)
                }
                Text(body, style = MaterialTheme.typography.bodySmall, color = if (isError) colors.error else colors.onSurfaceVariant)
            }
            TextButton(onClick = onAction) { Text(action) }
        }
    }
}

@Composable
private fun ConnectPrompt(onConnect: () -> Unit) {
    val colors = MaterialTheme.colorScheme
    Column(
        Modifier
            .fillMaxSize()
            .padding(S.xl),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Column(
            Modifier.weight(1f).fillMaxWidth().testTag(CONNECT_PROMPT_TAG),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Icon(
                painterResource(R.drawable.ic_launcher_foreground),
                contentDescription = null,
                tint = colors.onBackground,
                modifier = Modifier.size(S.appIcon + S.xxl),
            )
            Spacer(Modifier.height(S.lg))
            Text(
                stringResource(R.string.apps_connect_title),
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Bold,
                color = colors.onBackground,
                textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(S.sm))
            Text(
                stringResource(R.string.apps_connect_body),
                style = MaterialTheme.typography.bodyLarge,
                color = colors.onSurfaceVariant,
                textAlign = TextAlign.Center,
            )
        }
        Button(
            onClick = onConnect,
            shape = RoundedCornerShape(S.cornerRadius),
            modifier = Modifier.fillMaxWidth().heightIn(min = S.touchTarget + S.sm),
        ) {
            Text(stringResource(R.string.apps_connect_action), style = MaterialTheme.typography.titleSmall)
        }
    }
}
