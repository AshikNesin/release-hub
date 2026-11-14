package groot.mobile.ui

import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import androidx.core.net.toUri
import groot.mobile.BuildConfig
import groot.mobile.R

/** The maker's site with `?ref=<application id>`, so visits can be traced to the app that sent them. */
@Composable
fun makerUrl(): String = withRef(stringResource(R.string.config_maker_url), BuildConfig.APPLICATION_ID)

fun withRef(url: String, ref: String): String =
    url.toUri().buildUpon().appendQueryParameter("ref", ref).build().toString()
