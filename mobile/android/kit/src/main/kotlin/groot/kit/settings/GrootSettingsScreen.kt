package groot.kit.settings

import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import groot.kit.R
import groot.kit.buildinfo.BuildInfo
import groot.kit.theme.S
import groot.kit.theme.ThemeMode

/** What the header, About section and footer show. Set by the app; the kit has no app-specific copy. */
@Immutable
public data class SettingsAbout(
    val appName: String,
    val buildInfo: BuildInfo,
    /** Adds "Send feedback", opening a mail draft with the version in the subject. */
    val feedbackEmail: String? = null,
    /** Adds "Rate the app", opening this store URL. */
    val rateUrl: String? = null,
    /** Adds "Privacy policy", opening this URL. */
    val privacyPolicyUrl: String? = null,
    /** Adds the "Built by <maker>" footer. */
    val makerName: String? = null,
    /** Makes the footer open this URL. */
    val makerUrl: String? = null,
)

/**
 * The kit's settings screen (RFC 0001 §4): a large collapsing title, the app's own sections,
 * then Appearance (theme), About (version, or an "About" row via [onOpenAbout], rate, feedback, privacy policy),
 * Data (reset, with confirmation), an optional privacy note and a footer.
 *
 * Each section is one quiet card of rows (see SettingsComponents). Stateless: the host passes state and
 * persists changes. Build [appSections] from [SettingsSection] and the settings rows.
 * Need a different screen? Copy this into your app and stop calling it (RFC 0001 §5.2 rule 5).
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
public fun GrootSettingsScreen(
    themeMode: ThemeMode,
    onThemeModeChange: (ThemeMode) -> Unit,
    about: SettingsAbout,
    modifier: Modifier = Modifier,
    onNavigateUp: (() -> Unit)? = null,
    onOpenAbout: (() -> Unit)? = null,
    onResetData: (() -> Unit)? = null,
    privacyNote: String? = null,
    appSections: @Composable ColumnScope.() -> Unit = {},
) {
    val scrollBehavior = TopAppBarDefaults.exitUntilCollapsedScrollBehavior()
    var confirmReset by remember { mutableStateOf(false) }
    var mailUnavailable by remember { mutableStateOf(false) }
    val uriHandler = LocalUriHandler.current
    val open: (String) -> Unit = { url -> runCatching { uriHandler.openUri(url) } }
    val build = about.buildInfo

    Scaffold(
        modifier = modifier.nestedScroll(scrollBehavior.nestedScrollConnection),
        containerColor = MaterialTheme.colorScheme.background,
        topBar = {
            LargeTopAppBar(
                title = {
                    Text(
                        stringResource(R.string.groot_kit_settings),
                        fontWeight = FontWeight.Bold,
                        letterSpacing = (-0.5).sp,
                    )
                },
                navigationIcon = {
                    if (onNavigateUp != null) {
                        IconButton(onClick = onNavigateUp) {
                            Icon(
                                painter = painterResource(R.drawable.groot_kit_ic_arrow_back),
                                contentDescription = stringResource(R.string.groot_kit_navigate_up),
                            )
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.background,
                    scrolledContainerColor = MaterialTheme.colorScheme.background,
                ),
                scrollBehavior = scrollBehavior,
            )
        },
    ) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .verticalScroll(rememberScrollState())
                .padding(start = S.lg, end = S.lg, top = S.xs, bottom = S.xxl),
            verticalArrangement = Arrangement.spacedBy(S.xl),
        ) {
            appSections()

            SettingsSection(title = stringResource(R.string.groot_kit_appearance)) {
                SettingsChoiceRow(
                    title = stringResource(R.string.groot_kit_theme),
                    icon = painterResource(R.drawable.groot_kit_ic_theme),
                    options = listOf(
                        SettingsChoice(
                            ThemeMode.System,
                            stringResource(R.string.groot_kit_theme_system),
                            stringResource(R.string.groot_kit_theme_system_summary),
                        ),
                        SettingsChoice(ThemeMode.Light, stringResource(R.string.groot_kit_theme_light)),
                        SettingsChoice(ThemeMode.Dark, stringResource(R.string.groot_kit_theme_dark)),
                    ),
                    selected = themeMode,
                    onSelect = onThemeModeChange,
                )
            }

            val feedbackSubject = stringResource(R.string.groot_kit_feedback_subject, about.appName, build.versionName)
            run {
                SettingsSection(title = stringResource(R.string.groot_kit_about)) {
                    val version = stringResource(R.string.groot_kit_version_value, build.versionName, build.versionCode.toInt())
                    if (onOpenAbout != null) {
                        SettingsNavigationRow(
                            title = stringResource(R.string.groot_kit_about),
                            value = build.versionName,
                            icon = painterResource(R.drawable.groot_kit_ic_info),
                            onClick = onOpenAbout,
                        )
                    } else {
                        SettingsInfoRow(
                            title = stringResource(R.string.groot_kit_version),
                            value = version,
                            icon = painterResource(R.drawable.groot_kit_ic_info),
                        )
                    }
                    if (about.rateUrl != null) {
                        SettingsLinkRow(
                            title = stringResource(R.string.groot_kit_rate),
                            icon = painterResource(R.drawable.groot_kit_ic_star),
                            onClick = { open(about.rateUrl) },
                        )
                    }
                    if (about.feedbackEmail != null) {
                        SettingsLinkRow(
                            title = stringResource(R.string.groot_kit_feedback),
                            icon = painterResource(R.drawable.groot_kit_ic_mail),
                            onClick = {
                                val uri = "mailto:${about.feedbackEmail}?subject=${Uri.encode(feedbackSubject)}"
                                runCatching { uriHandler.openUri(uri) }.onFailure { mailUnavailable = true }
                            },
                        )
                    }
                    if (about.privacyPolicyUrl != null) {
                        SettingsLinkRow(
                            title = stringResource(R.string.groot_kit_privacy_policy),
                            icon = painterResource(R.drawable.groot_kit_ic_policy),
                            onClick = { open(about.privacyPolicyUrl) },
                        )
                    }
                }
            }

            if (onResetData != null) {
                SettingsSection(title = stringResource(R.string.groot_kit_data)) {
                    SettingsActionRow(
                        title = stringResource(R.string.groot_kit_reset),
                        icon = painterResource(R.drawable.groot_kit_ic_delete),
                        destructive = true,
                        onClick = { confirmReset = true },
                    )
                }
            }

            if (privacyNote != null || about.makerName != null) {
                Column(verticalArrangement = Arrangement.spacedBy(S.xs)) {
                    if (privacyNote != null) SettingsNote(text = privacyNote)
                    if (about.makerName != null) {
                        SettingsFooter(
                            text = stringResource(R.string.groot_kit_footer, about.makerName),
                            onClick = about.makerUrl?.let { url -> { open(url) } },
                        )
                    }
                }
            }
        }
    }

    if (confirmReset && onResetData != null) {
        AlertDialog(
            onDismissRequest = { confirmReset = false },
            title = { Text(stringResource(R.string.groot_kit_reset_confirm_title)) },
            text = { Text(stringResource(R.string.groot_kit_reset_confirm_body)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmReset = false
                    onResetData()
                }) {
                    Text(stringResource(R.string.groot_kit_reset_confirm), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = {
                TextButton(onClick = { confirmReset = false }) { Text(stringResource(R.string.groot_kit_cancel)) }
            },
        )
    }

    if (mailUnavailable && about.feedbackEmail != null) {
        AlertDialog(
            onDismissRequest = { mailUnavailable = false },
            title = { Text(stringResource(R.string.groot_kit_mail_unavailable_title)) },
            text = { Text(stringResource(R.string.groot_kit_mail_unavailable_body, about.feedbackEmail)) },
            confirmButton = {
                TextButton(onClick = { mailUnavailable = false }) { Text(stringResource(R.string.groot_kit_ok)) }
            },
        )
    }
}
