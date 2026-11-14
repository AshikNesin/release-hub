package groot.mobile.navigation

import androidx.compose.animation.AnimatedContentTransitionScope
import androidx.compose.animation.EnterTransition
import androidx.compose.animation.ExitTransition
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.tween
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.shadow
import groot.kit.theme.S
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavBackStackEntry
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import groot.kit.buildinfo.BuildInfo
import groot.kit.theme.ThemeModeStore
import groot.mobile.data.AppPreferences
import groot.mobile.feature.about.AboutScreen
import groot.mobile.feature.components.ComponentsScreen
import groot.mobile.feature.home.HomeScreen
import groot.mobile.feature.settings.SettingsRoute
import groot.mobile.feature.settings.SettingsViewModel

object Routes {
    const val HOME = "home"
    const val SETTINGS = "settings"
    const val ABOUT = "about"
    const val COMPONENTS = "components"
}

/*
 * One minimal transition for every screen: only the screen being opened or closed moves.
 * It slides in from the end over a still background, and slides back out on back.
 * No fades, no parallax, nothing moving behind it; a soft edge shadow (ScreenContainer) marks the sheet.
 */
private const val TRANSITION_MS = 250
private val easing = FastOutSlowInEasing

private fun AnimatedContentTransitionScope<NavBackStackEntry>.enter(): EnterTransition =
    slideInHorizontally(tween(TRANSITION_MS, easing = easing)) { it }

private fun AnimatedContentTransitionScope<NavBackStackEntry>.exit(): ExitTransition =
    ExitTransition.KeepUntilTransitionsFinished

private fun AnimatedContentTransitionScope<NavBackStackEntry>.popEnter(): EnterTransition =
    EnterTransition.None

private fun AnimatedContentTransitionScope<NavBackStackEntry>.popExit(): ExitTransition =
    slideOutHorizontally(tween(TRANSITION_MS, easing = easing)) { it }

@Composable
fun AppNavHost(
    themeModeStore: ThemeModeStore,
    appPreferences: AppPreferences,
    buildInfo: () -> BuildInfo,
    navController: NavHostController = rememberNavController(),
) {
    NavHost(
        navController = navController,
        startDestination = Routes.HOME,
        enterTransition = { enter() },
        exitTransition = { exit() },
        popEnterTransition = { popEnter() },
        popExitTransition = { popExit() },
        // Gesture back (predictive back, on by default when targeting Android 16) has its own
        // transitions in Navigation 2.10 and defaults to shrinking the screen into a card.
        // Use the same slide so swiping back looks like pressing back, following the finger.
        predictivePopEnterTransition = { popEnter() },
        predictivePopExitTransition = { popExit() },
    ) {
        composable(Routes.HOME) {
            ScreenContainer {
            val showHomeIntro by appPreferences.showHomeIntro.collectAsStateWithLifecycle(initialValue = true)
            HomeScreen(
                showHomeIntro = showHomeIntro,
                onOpenComponents = { navController.navigate(Routes.COMPONENTS) },
                onOpenSettings = { navController.navigate(Routes.SETTINGS) },
            )
            }
        }
        composable(Routes.SETTINGS) {
            ScreenContainer {
            val viewModel: SettingsViewModel = viewModel(factory = SettingsViewModel.factory(themeModeStore, appPreferences))
            SettingsRoute(
                viewModel = viewModel,
                buildInfo = buildInfo(),
                onNavigateUp = navController::navigateUp,
                onOpenAbout = { navController.navigate(Routes.ABOUT) },
            )
            }
        }
        composable(Routes.COMPONENTS) {
            ScreenContainer {
                ComponentsScreen(
                    onNavigateUp = navController::navigateUp,
                    onOpenSettings = { navController.navigate(Routes.SETTINGS) },
                )
            }
        }
        composable(Routes.ABOUT) {
            ScreenContainer { AboutScreen(buildInfo = buildInfo(), onNavigateUp = navController::navigateUp) }
        }
    }
}

/**
 * Gives each screen an opaque background and a soft shadow, so a sliding screen reads as a
 * sheet over the one behind. At rest the shadow sits outside the screen and isn't visible.
 */
@Composable
private fun ScreenContainer(content: @Composable () -> Unit) {
    Box(
        Modifier
            .fillMaxSize()
            .shadow(S.md)
            .background(MaterialTheme.colorScheme.background),
    ) {
        content()
    }
}
