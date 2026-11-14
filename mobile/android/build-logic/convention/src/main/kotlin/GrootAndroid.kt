import org.gradle.api.JavaVersion
import org.gradle.api.Project
import org.gradle.api.artifacts.VersionCatalog
import org.gradle.api.artifacts.VersionCatalogsExtension
import org.gradle.kotlin.dsl.getByType
import org.gradle.kotlin.dsl.withType
import org.jetbrains.kotlin.gradle.dsl.JvmTarget
import org.jetbrains.kotlin.gradle.tasks.KotlinCompile

/** Single place for SDK levels and JVM targets shared by every module. */
internal object GrootSdk {
    const val COMPILE = 37
    const val TARGET = 36
    const val MIN = 29
    val JAVA = JavaVersion.VERSION_17
    val JVM = JvmTarget.JVM_17
}

internal val Project.libs: VersionCatalog
    get() = extensions.getByType<VersionCatalogsExtension>().named("libs")

internal fun VersionCatalog.lib(alias: String) = findLibrary(alias).get()

internal fun Project.configureKotlin(explicitApi: Boolean = false) {
    tasks.withType<KotlinCompile>().configureEach {
        compilerOptions {
            jvmTarget.set(GrootSdk.JVM)
            // Explicit API mode for main sources only, so test code stays terse.
            if (explicitApi && !name.contains("Test")) {
                freeCompilerArgs.add("-Xexplicit-api=strict")
            }
        }
    }
}
