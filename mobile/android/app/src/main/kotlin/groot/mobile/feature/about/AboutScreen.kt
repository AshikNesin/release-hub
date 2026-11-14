package groot.mobile.feature.about

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import groot.kit.buildinfo.BuildInfo
import groot.kit.settings.SettingsAppHeader
import groot.kit.settings.SettingsFooter
import groot.kit.settings.SettingsInfoRow
import groot.kit.settings.SettingsSection
import groot.kit.theme.S
import groot.mobile.R
import groot.mobile.ui.makerUrl
import groot.mobile.ui.components.GrootTopBar

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AboutScreen(buildInfo: BuildInfo, onNavigateUp: () -> Unit) {
    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    Scaffold(
        modifier = Modifier.nestedScroll(scrollBehavior.nestedScrollConnection),
        topBar = { GrootTopBar(title = stringResource(R.string.about_title), scrollBehavior = scrollBehavior, onNavigateUp = onNavigateUp) },
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
            SettingsAppHeader(
                appName = stringResource(R.string.app_name),
                versionLine = stringResource(R.string.about_tagline),
                icon = painterResource(R.drawable.ic_launcher_foreground),
            )
            SettingsSection(title = stringResource(R.string.about_section_build)) {
                SettingsInfoRow(stringResource(R.string.about_version), buildInfo.versionName, painterResource(R.drawable.ic_info))
                SettingsInfoRow(stringResource(R.string.about_build), buildInfo.versionCode.toString(), painterResource(R.drawable.ic_tag))
                SettingsInfoRow(stringResource(R.string.about_commit), buildInfo.commit, painterResource(R.drawable.ic_code))
                SettingsInfoRow(
                    stringResource(R.string.about_build_type),
                    stringResource(if (buildInfo.isDebug) R.string.about_build_type_debug else R.string.about_build_type_release),
                    painterResource(R.drawable.ic_tune),
                )
            }
            val uriHandler = LocalUriHandler.current
            val makerUrl = makerUrl()
            SettingsFooter(
                text = stringResource(R.string.about_footer, stringResource(R.string.config_maker_name)),
                onClick = { runCatching { uriHandler.openUri(makerUrl) } },
            )
        }
    }
}
