import com.android.build.api.dsl.LibraryExtension
import org.gradle.api.Plugin
import org.gradle.api.Project
import org.gradle.kotlin.dsl.configure

/** groot.android.library — SDK levels, consumer R8 rules, explicit API mode, lint. */
class AndroidLibraryConventionPlugin : Plugin<Project> {
    override fun apply(target: Project) = with(target) {
        pluginManager.apply("com.android.library")
        extensions.configure<LibraryExtension> {
            compileSdk = GrootSdk.COMPILE
            defaultConfig {
                minSdk = GrootSdk.MIN
                consumerProguardFiles("consumer-rules.pro")
            }
            compileOptions {
                sourceCompatibility = GrootSdk.JAVA
                targetCompatibility = GrootSdk.JAVA
            }
            lint {
                abortOnError = true
                warningsAsErrors = false
            }
        }
        configureKotlin(explicitApi = true)
    }
}
