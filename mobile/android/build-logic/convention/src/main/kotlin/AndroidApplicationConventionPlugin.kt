import com.android.build.api.dsl.ApplicationExtension
import org.gradle.api.Plugin
import org.gradle.api.Project
import org.gradle.kotlin.dsl.configure

/** groot.android.application — SDK levels, R8 full mode + resource shrinking, lint. */
class AndroidApplicationConventionPlugin : Plugin<Project> {
    override fun apply(target: Project) = with(target) {
        pluginManager.apply("com.android.application")
        extensions.configure<ApplicationExtension> {
            compileSdk = GrootSdk.COMPILE
            defaultConfig {
                minSdk = GrootSdk.MIN
                targetSdk = GrootSdk.TARGET
            }
            compileOptions {
                sourceCompatibility = GrootSdk.JAVA
                targetCompatibility = GrootSdk.JAVA
            }
            buildTypes {
                getByName("release") {
                    isMinifyEnabled = true
                    isShrinkResources = true
                    proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
                }
            }
            lint {
                abortOnError = true
                checkDependencies = true
                warningsAsErrors = false
            }
        }
        configureKotlin()
    }
}
