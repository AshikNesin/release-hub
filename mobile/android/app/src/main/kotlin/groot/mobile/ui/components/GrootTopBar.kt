package groot.mobile.ui.components

import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LargeTopAppBar
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.TopAppBarScrollBehavior
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import groot.mobile.R

/** Large collapsing title, matching the kit's settings screen. Pass [onNavigateUp] for a back button. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GrootTopBar(
    title: String,
    scrollBehavior: TopAppBarScrollBehavior,
    onNavigateUp: (() -> Unit)? = null,
) {
    LargeTopAppBar(
        title = { Text(title, fontWeight = FontWeight.Bold, letterSpacing = (-0.5).sp) },
        navigationIcon = {
            if (onNavigateUp != null) {
                IconButton(onClick = onNavigateUp) {
                    Icon(
                        painter = painterResource(R.drawable.ic_arrow_back),
                        contentDescription = stringResource(R.string.navigate_up),
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
}
