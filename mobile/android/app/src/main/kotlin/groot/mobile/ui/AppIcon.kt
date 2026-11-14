package groot.mobile.ui

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import groot.kit.theme.S
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** Loads an installed app's launcher icon; returns null for apps that aren't installed. */
fun interface AppIconLoader {
    fun load(packageName: String): ImageBitmap?

    companion object {
        val None = AppIconLoader { null }
    }
}

/** The installed app's icon, or its first letter on a tile when it isn't installed. */
@Composable
fun AppIcon(
    packageName: String,
    title: String,
    loader: AppIconLoader,
    size: Dp,
    modifier: Modifier = Modifier,
    /** Changes when the installed version changes, so a new icon is picked up after an update. */
    key: Any? = null,
) {
    val icon by produceState<ImageBitmap?>(null, packageName, key) {
        value = withContext(Dispatchers.IO) { loader.load(packageName) }
    }
    val bitmap = icon
    if (bitmap != null) {
        Image(bitmap = bitmap, contentDescription = null, modifier = modifier.size(size))
    } else {
        val colors = MaterialTheme.colorScheme
        Box(
            modifier
                .size(size)
                .clip(RoundedCornerShape(S.md))
                .background(colors.surfaceContainerHigh),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                title.firstOrNull()?.uppercase() ?: "?",
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Bold,
                color = colors.onSurfaceVariant,
            )
        }
    }
}
