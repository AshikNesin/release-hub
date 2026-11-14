package groot.mobile

import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import groot.kit.theme.GrootTheme
import groot.kit.theme.ThemeMode
import groot.mobile.navigation.AppNavHost
import groot.mobile.ui.brand.BrandPalette
import kotlinx.coroutines.runBlocking

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        val container = (application as GrootMobileApp).container
        val store = container.themeModeStore

        // API 31+: ThemeModeStore mirrored the saved mode to UiModeManager, so the configuration
        // is already right and "System" renders the correct first frame. API 29–30: read the one
        // key before the first frame so a dark override never flashes light (RFC 0001 Q4).
        val initialMode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            ThemeMode.System
        } else {
            runBlocking { store.current() }
        }

        setContent {
            val themeMode by store.themeMode.collectAsStateWithLifecycle(initialValue = initialMode)
            GrootTheme(mode = themeMode, palette = BrandPalette) {
                AppNavHost(
                    themeModeStore = store,
                    appPreferences = container.appPreferences,
                    buildInfo = { container.buildInfo },
                )
            }
        }
    }
}
