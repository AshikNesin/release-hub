package groot.kit.startup

import android.app.Activity
import android.content.Context
import android.content.ContextWrapper
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.platform.LocalView

/**
 * Marks the first meaningful frame, so time to full display (TTFD) is measurable by
 * macrobenchmark and Play vitals. Place it in the first screen once its real content is
 * shown; it reports once per activity.
 */
@Composable
public fun ReportFullyDrawn() {
    val view = LocalView.current
    LaunchedEffect(Unit) {
        withFrameNanos { }
        view.context.findActivity()?.reportFullyDrawn()
    }
}

private tailrec fun Context.findActivity(): Activity? = when (this) {
    is Activity -> this
    is ContextWrapper -> baseContext.findActivity()
    else -> null
}
