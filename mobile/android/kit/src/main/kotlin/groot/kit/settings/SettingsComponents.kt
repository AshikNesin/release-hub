package groot.kit.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.RadioButton
import androidx.compose.material3.RadioButtonDefaults
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.painter.Painter
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Constraints
import androidx.compose.ui.unit.em
import androidx.compose.ui.unit.sp
import groot.kit.R
import groot.kit.theme.GrootTheme
import groot.kit.theme.S
import kotlinx.coroutines.launch

/*
 * Minimal grouped-list building blocks in tiny-firewall's style (RFC 0001 §4.1): small all-caps
 * grey section labels, borderless cards a step off the page, hairline dividers between rows,
 * grey icons, values on the right. White (black in light mode) is the only accent: switches,
 * the selected option. Picking one option from a list always opens a bottom sheet.
 * Use them for your app's own sections so everything matches.
 */

/** A labelled card of rows. Dividers between rows are drawn for you. */
@Composable
public fun SettingsSection(
    title: String?,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    Column(modifier = modifier.fillMaxWidth()) {
        if (title != null) {
            Text(
                text = title.uppercase(),
                style = MaterialTheme.typography.labelMedium,
                fontWeight = FontWeight.Normal,
                letterSpacing = SECTION_LABEL_TRACKING,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier
                    .padding(start = S.xs, bottom = S.md)
                    .semantics { heading() },
            )
        }
        Surface(
            color = GrootTheme.colors.cardSurface,
            shape = RoundedCornerShape(S.cornerRadius),
            modifier = Modifier.fillMaxWidth(),
        ) {
            DividedColumn(content)
        }
    }
}

/** A row that opens another screen in the app. [value] shows on the right; [summary] below the title. */
@Composable
public fun SettingsNavigationRow(
    title: String,
    onClick: () -> Unit,
    icon: Painter? = null,
    summary: String? = null,
    value: String? = null,
) {
    RowContent(
        icon = icon,
        title = title,
        summary = summary,
        value = value,
        modifier = Modifier.clickable(role = Role.Button, onClick = onClick),
    ) {
        TrailingIcon(painterResource(R.drawable.groot_kit_ic_chevron_right))
    }
}

/** A row that leaves the app (browser, store, mail), marked with an outward arrow. */
@Composable
public fun SettingsLinkRow(
    title: String,
    onClick: () -> Unit,
    icon: Painter? = null,
    summary: String? = null,
) {
    RowContent(
        icon = icon,
        title = title,
        summary = summary,
        modifier = Modifier.clickable(role = Role.Button, onClick = onClick),
    ) {
        TrailingIcon(painterResource(R.drawable.groot_kit_ic_open_external))
    }
}

/** An on/off setting. The whole row toggles. */
@Composable
public fun SettingsSwitchRow(
    title: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    icon: Painter? = null,
    summary: String? = null,
    enabled: Boolean = true,
) {
    RowContent(
        icon = icon,
        title = title,
        summary = summary,
        enabled = enabled,
        modifier = Modifier.toggleable(value = checked, enabled = enabled, role = Role.Switch, onValueChange = onCheckedChange),
    ) {
        val c = MaterialTheme.colorScheme
        val offThumb = if (GrootTheme.isDark) c.onSurface else c.surface
        Switch(
            checked = checked,
            onCheckedChange = null,
            enabled = enabled,
            colors = SwitchDefaults.colors(
                checkedThumbColor = c.onPrimary,
                checkedTrackColor = c.primary,
                checkedBorderColor = c.primary,
                uncheckedThumbColor = offThumb,
                uncheckedTrackColor = c.outline,
                uncheckedBorderColor = Color.Transparent,
                disabledCheckedThumbColor = c.onPrimary,
                disabledCheckedTrackColor = c.primary,
                disabledUncheckedThumbColor = offThumb,
                disabledUncheckedTrackColor = c.outline,
                disabledUncheckedBorderColor = Color.Transparent,
            ),
        )
    }
}

/** A read-only row: a fact with its [value] on the right, or a line of text with a [summary]. */
@Composable
public fun SettingsInfoRow(
    title: String,
    value: String? = null,
    icon: Painter? = null,
    summary: String? = null,
) {
    RowContent(icon = icon, title = title, summary = summary, value = value)
}

/** A one-shot action. [destructive] colors it as a danger action. */
@Composable
public fun SettingsActionRow(
    title: String,
    onClick: () -> Unit,
    icon: Painter? = null,
    summary: String? = null,
    destructive: Boolean = false,
) {
    RowContent(
        icon = icon,
        title = title,
        summary = summary,
        accent = if (destructive) MaterialTheme.colorScheme.error else null,
        modifier = Modifier.clickable(role = Role.Button, onClick = onClick),
    )
}

/** One option of a [SettingsChoiceRow]. Unavailable options stay listed, faded, and can't be picked. */
@Immutable
public data class SettingsChoice<T>(
    val value: T,
    val label: String,
    val summary: String? = null,
    val enabled: Boolean = true,
)

/**
 * Pick one option from a list. The row shows the current choice on the right; tapping it opens
 * [SettingsChoiceSheet] from the bottom. Picking an option applies it and closes the sheet.
 */
@Composable
public fun <T> SettingsChoiceRow(
    title: String,
    options: List<SettingsChoice<T>>,
    selected: T,
    onSelect: (T) -> Unit,
    icon: Painter? = null,
    summary: String? = null,
    sheetTitle: String = title,
    enabled: Boolean = true,
) {
    var open by rememberSaveable { mutableStateOf(false) }
    val selectedLabel = options.firstOrNull { it.value == selected }?.label.orEmpty()
    RowContent(
        icon = icon,
        title = title,
        summary = summary,
        value = selectedLabel,
        enabled = enabled,
        modifier = Modifier.clickable(enabled = enabled, role = Role.Button) { open = true },
    )
    if (open) {
        SettingsChoiceSheet(
            title = sheetTitle,
            options = options,
            selected = selected,
            onSelect = onSelect,
            onDismissRequest = { open = false },
        )
    }
}

/**
 * A bottom sheet with one radio row per option, as in tiny-firewall's exit-node picker. Selecting
 * an option calls [onSelect], slides the sheet away, then calls [onDismissRequest]. Swiping down or
 * tapping outside only dismisses. Long lists scroll inside the sheet.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
public fun <T> SettingsChoiceSheet(
    title: String,
    options: List<SettingsChoice<T>>,
    selected: T,
    onSelect: (T) -> Unit,
    onDismissRequest: () -> Unit,
) {
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    val scope = rememberCoroutineScope()
    ModalBottomSheet(
        onDismissRequest = onDismissRequest,
        sheetState = sheetState,
        shape = RoundedCornerShape(topStart = S.cornerRadius, topEnd = S.cornerRadius),
        containerColor = GrootTheme.colors.cardSurface,
        contentColor = MaterialTheme.colorScheme.onSurface,
        scrimColor = MaterialTheme.colorScheme.scrim.copy(alpha = SCRIM_ALPHA),
        dragHandle = null,
    ) {
        Column(Modifier.padding(top = S.lg, bottom = S.sm)) {
            Text(
                title,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Bold,
                color = MaterialTheme.colorScheme.onSurface,
                modifier = Modifier
                    .padding(horizontal = S.xl)
                    .padding(bottom = S.sm)
                    .semantics { heading() },
            )
            LazyColumn(Modifier.selectableGroup()) {
                items(options) { option ->
                    ChoiceOption(option, selected = option.value == selected) {
                        onSelect(option.value)
                        scope.launch { sheetState.hide() }.invokeOnCompletion { onDismissRequest() }
                    }
                }
            }
        }
    }
}

/** The app's identity, centered: icon, name and one line under it. For About screens. */
@Composable
public fun SettingsAppHeader(
    appName: String,
    versionLine: String,
    modifier: Modifier = Modifier,
    icon: Painter? = null,
) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = S.lg),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(S.xs),
    ) {
        if (icon != null) {
            Box(
                Modifier
                    .padding(bottom = S.md)
                    .size(S.appIcon)
                    .clip(RoundedCornerShape(S.cornerRadius))
                    .background(MaterialTheme.colorScheme.primary),
                contentAlignment = Alignment.Center,
            ) {
                Icon(painter = icon, contentDescription = null, tint = MaterialTheme.colorScheme.onPrimary, modifier = Modifier.size(S.appIcon))
            }
        }
        Text(appName, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.onSurface)
        Text(
            versionLine,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
        )
    }
}

/** A quiet centered note under the sections, e.g. a privacy promise. */
@Composable
public fun SettingsNote(
    text: String,
    modifier: Modifier = Modifier,
) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        textAlign = TextAlign.Center,
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = S.xl),
    )
}

/** Small centered footer, e.g. "App v1.0 · Built by …". Tappable when [onClick] is set. */
@Composable
public fun SettingsFooter(
    text: String,
    modifier: Modifier = Modifier,
    onClick: (() -> Unit)? = null,
) {
    Text(
        text = text,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        textAlign = TextAlign.Center,
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(S.sm))
            .then(if (onClick != null) Modifier.clickable(role = Role.Button, onClick = onClick) else Modifier)
            .padding(S.sm),
    )
}

/** Stacks rows and draws a hairline between them, inset from the start like a list. */
@Composable
private fun DividedColumn(content: @Composable () -> Unit) {
    val dividerColor = MaterialTheme.colorScheme.outlineVariant
    val dividerInset = S.lg + S.icon + S.md + S.xs
    val boundaries = remember { mutableListOf<Int>() }
    Layout(
        content = content,
        modifier = Modifier.drawWithContent {
            drawContent()
            val inset = dividerInset.toPx()
            val stroke = 1f
            boundaries.forEach { y ->
                drawLine(dividerColor, Offset(inset, y.toFloat()), Offset(size.width, y.toFloat()), strokeWidth = stroke)
            }
        },
    ) { measurables, constraints ->
        val placeables = measurables.map { it.measure(constraints.copy(minHeight = 0)) }
        val height = placeables.sumOf { it.height }
        layout(constraints.maxWidth.takeIf { it != Constraints.Infinity } ?: placeables.maxOfOrNull { it.width } ?: 0, height) {
            boundaries.clear()
            var y = 0
            placeables.forEachIndexed { index, placeable ->
                if (index > 0) boundaries += y
                placeable.place(0, y)
                y += placeable.height
            }
        }
    }
}

@Composable
private fun RowContent(
    icon: Painter?,
    title: String,
    modifier: Modifier = Modifier,
    summary: String? = null,
    value: String? = null,
    enabled: Boolean = true,
    accent: Color? = null,
    trailing: (@Composable RowScope.() -> Unit)? = null,
) {
    val alpha = if (enabled) 1f else UNAVAILABLE_ALPHA
    Row(
        modifier = modifier
            .fillMaxWidth()
            .heightIn(min = S.touchTarget + S.sm)
            .padding(horizontal = S.lg, vertical = S.md),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(S.md),
    ) {
        if (icon != null) {
            Icon(
                painter = icon,
                contentDescription = null,
                tint = (accent ?: MaterialTheme.colorScheme.onSurfaceVariant).copy(alpha = alpha),
                modifier = Modifier.size(S.icon),
            )
            Spacer(Modifier.width(S.xs))
        }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(S.xxs)) {
            Text(
                title,
                style = rowTitleStyle(),
                color = (accent ?: MaterialTheme.colorScheme.onSurface).copy(alpha = alpha),
            )
            if (summary != null) {
                Text(
                    summary,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = alpha),
                )
            }
        }
        if (value != null) {
            Text(
                value,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = alpha),
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        trailing?.invoke(this)
    }
}

@Composable
private fun <T> ChoiceOption(option: SettingsChoice<T>, selected: Boolean, onClick: () -> Unit) {
    val c = MaterialTheme.colorScheme
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .selectable(selected = selected, enabled = option.enabled, role = Role.RadioButton, onClick = onClick)
            .alpha(if (option.enabled) 1f else UNAVAILABLE_ALPHA)
            .heightIn(min = S.touchTarget + S.sm)
            .padding(horizontal = S.xl, vertical = S.sm),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(S.md),
    ) {
        RadioButton(
            selected = selected,
            onClick = null,
            enabled = option.enabled,
            colors = RadioButtonDefaults.colors(
                selectedColor = c.primary,
                unselectedColor = c.primary,
                disabledSelectedColor = c.primary,
                disabledUnselectedColor = c.primary,
            ),
        )
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(S.xxs)) {
            Text(option.label, style = rowTitleStyle(), color = c.onSurface, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (option.summary != null) {
                Text(option.summary, style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant)
            }
        }
    }
}

@Composable
private fun TrailingIcon(painter: Painter, modifier: Modifier = Modifier) {
    Icon(painter = painter, contentDescription = null, tint = MaterialTheme.colorScheme.outline, modifier = modifier.size(S.icon))
}

/** Row titles: 15 sp, between Material's body sizes, as in tiny-firewall. */
@Composable
@ReadOnlyComposable
private fun rowTitleStyle(): TextStyle = MaterialTheme.typography.bodyLarge.copy(fontSize = ROW_TITLE_SIZE)

/** Opacity for items that are listed but unavailable right now (an offline peer, a setting that doesn't apply). */
private const val UNAVAILABLE_ALPHA = 0.4f
private const val SCRIM_ALPHA = 0.6f
private val ROW_TITLE_SIZE = 15.sp
private val SECTION_LABEL_TRACKING = 0.08.em
