package groot.mobile.feature.apps

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import groot.kit.theme.S
import groot.mobile.R
import groot.mobile.data.catalog.AppStatus
import groot.mobile.data.catalog.CatalogEntry
import groot.mobile.install.InstallState
import groot.mobile.ui.AppIcon
import groot.mobile.ui.AppIconLoader
import groot.mobile.ui.versionLabel

/** What the row's button does. */
enum class RowAction { Install, Update, Open }

fun primaryAction(entry: CatalogEntry, canOpen: Boolean): RowAction? = when (entry.status) {
    AppStatus.UpdateAvailable -> RowAction.Update
    AppStatus.NotInstalled -> RowAction.Install
    AppStatus.UpToDate, AppStatus.AheadOfHub -> if (canOpen && !entry.isSelf) RowAction.Open else null
    AppStatus.NoRelease -> if (canOpen && entry.installed != null && !entry.isSelf) RowAction.Open else null
}

/** One app in a section card: icon, name, version line or install progress, and one action. */
@Composable
fun AppRow(
    entry: CatalogEntry,
    install: InstallState?,
    icons: AppIconLoader,
    canOpen: Boolean,
    onClick: () -> Unit,
    onAction: (RowAction) -> Unit,
) {
    val colors = MaterialTheme.colorScheme
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .heightIn(min = S.touchTarget + S.md)
            .clickable(onClick = onClick)
            .padding(horizontal = S.lg, vertical = S.md),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        AppIcon(entry.packageName, entry.title, icons, size = S.xxl + S.sm, key = entry.installed?.versionCode)
        Spacer(Modifier.width(S.md))
        Column(Modifier.weight(1f)) {
            Text(
                entry.title,
                style = MaterialTheme.typography.bodyLarge,
                color = colors.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            val failed = install as? InstallState.Failed
            Text(
                failed?.message ?: subtitle(entry, install),
                style = MaterialTheme.typography.bodySmall,
                color = if (failed != null) colors.error else colors.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            if (install is InstallState.Downloading || install == InstallState.Installing || install == InstallState.Queued) {
                Spacer(Modifier.height(S.xs))
                val progress = (install as? InstallState.Downloading)?.progress
                if (progress != null) {
                    LinearProgressIndicator(progress = { progress }, modifier = Modifier.fillMaxWidth())
                } else {
                    LinearProgressIndicator(modifier = Modifier.fillMaxWidth())
                }
            }
        }
        val action = primaryAction(entry, canOpen)
        if (action != null && install?.busy != true) {
            Spacer(Modifier.width(S.sm))
            when (action) {
                RowAction.Open -> TextButton(onClick = { onAction(action) }) { Text(stringResource(R.string.action_open)) }
                RowAction.Install -> FilledTonalButton(onClick = { onAction(action) }) { Text(stringResource(R.string.action_install)) }
                RowAction.Update -> FilledTonalButton(onClick = { onAction(action) }) { Text(stringResource(R.string.action_update)) }
            }
        }
    }
}

@Composable
private fun subtitle(entry: CatalogEntry, install: InstallState?): String {
    when (install) {
        InstallState.Queued -> return stringResource(R.string.status_queued)
        is InstallState.Downloading -> return install.progress?.let {
            stringResource(R.string.status_downloading_percent, (it * 100).toInt())
        } ?: stringResource(R.string.status_downloading)
        InstallState.Installing -> return stringResource(R.string.status_installing)
        InstallState.WaitingForUser -> return stringResource(R.string.status_waiting_for_user)
        else -> Unit
    }
    val installed = entry.installed?.let { versionLabel(it.versionName, it.versionCode) }
    val latest = entry.latest?.let { versionLabel(it.versionName, it.versionCode) }
    val line = when (entry.status) {
        AppStatus.UpdateAvailable -> stringResource(R.string.status_update_line, installed.orEmpty(), latest.orEmpty())
        AppStatus.NotInstalled, AppStatus.UpToDate -> latest.orEmpty()
        AppStatus.AheadOfHub -> stringResource(R.string.status_ahead, installed.orEmpty(), latest.orEmpty())
        AppStatus.NoRelease -> entry.error ?: installed ?: stringResource(R.string.status_no_release)
    }
    return if (entry.isSelf) "${stringResource(R.string.apps_this_app)} · $line" else line
}
