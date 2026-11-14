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
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.navigation.NavBackStackEntry
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.NavType
import androidx.navigation.navArgument
import groot.mobile.feature.apps.AppsRoute
import groot.mobile.feature.apps.AppsViewModel
import groot.mobile.feature.connect.ConnectRoute
import groot.mobile.feature.connect.ConnectViewModel
import groot.mobile.feature.detail.AppDetailRoute
import groot.mobile.feature.detail.AppDetailViewModel
import groot.mobile.feature.about.AboutScreen
import groot.mobile.feature.settings.SettingsRoute
import groot.mobile.feature.settings.SettingsViewModel

object Routes {
    const val APPS = "apps"
    const val APP = "app/{slug}"
    const val CONNECT = "connect"
    const val SETTINGS = "settings"
    const val ABOUT = "about"

    fun app(slug: String) = "app/${android.net.Uri.encode(slug)}"
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
    graph: AppGraph,
    navController: NavHostController = rememberNavController(),
) {
    NavHost(
        navController = navController,
        startDestination = Routes.APPS,
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
        composable(Routes.APPS) {
            ScreenContainer {
                val viewModel: AppsViewModel = viewModel(
                    factory = AppsViewModel.factory(graph.hubSettings, graph.catalog, graph.installController, graph.installTracker, graph.appScope),
                )
                AppsRoute(
                    viewModel = viewModel,
                    installedApps = graph.installedApps,
                    icons = graph.icons,
                    onOpenApp = { navController.navigate(Routes.app(it)) },
                    onConnect = { navController.navigate(Routes.CONNECT) },
                    onOpenSettings = { navController.navigate(Routes.SETTINGS) },
                )
            }
        }
        composable(Routes.APP, arguments = listOf(navArgument("slug") { type = NavType.StringType })) { entry ->
            val slug = entry.arguments?.getString("slug").orEmpty()
            ScreenContainer {
                val viewModel: AppDetailViewModel = viewModel(
                    factory = AppDetailViewModel.factory(
                        slug, graph.hubSettings, graph.catalog, graph.installController, graph.installTracker, graph.appScope,
                    ),
                )
                AppDetailRoute(
                    viewModel = viewModel,
                    installedApps = graph.installedApps,
                    icons = graph.icons,
                    selfPackage = graph.selfPackage,
                    onNavigateUp = navController::navigateUp,
                )
            }
        }
        composable(Routes.CONNECT) {
            ScreenContainer {
                val viewModel: ConnectViewModel = viewModel(
                    factory = ConnectViewModel.factory(graph.hubApi, graph.hubSettings, graph.allowCleartext),
                )
                ConnectRoute(
                    viewModel = viewModel,
                    onConnected = { navController.popBackStack(Routes.APPS, inclusive = false) },
                    onNavigateUp = navController::navigateUp,
                )
            }
        }
        composable(Routes.SETTINGS) {
            ScreenContainer {
                val viewModel: SettingsViewModel = viewModel(
                    factory = SettingsViewModel.factory(graph.themeModeStore, graph.hubSettings, graph.catalog),
                )
                SettingsRoute(
                    viewModel = viewModel,
                    buildInfo = graph.buildInfo(),
                    canInstall = graph.installController::canInstall,
                    onNavigateUp = navController::navigateUp,
                    onOpenAbout = { navController.navigate(Routes.ABOUT) },
                    onOpenHub = { navController.navigate(Routes.CONNECT) },
                )
            }
        }
        composable(Routes.ABOUT) {
            ScreenContainer { AboutScreen(buildInfo = graph.buildInfo(), onNavigateUp = navController::navigateUp) }
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
