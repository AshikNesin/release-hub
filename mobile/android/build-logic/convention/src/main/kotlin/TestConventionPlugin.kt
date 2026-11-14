import com.android.build.api.dsl.ApplicationExtension
import com.android.build.api.dsl.LibraryExtension
import org.gradle.api.Plugin
import org.gradle.api.Project
import org.gradle.api.tasks.testing.Test
import org.gradle.kotlin.dsl.dependencies
import org.gradle.kotlin.dsl.withType

/** groot.test — JUnit + Robolectric + coroutines-test for src/test. No network, clock or disk in unit tests. */
class TestConventionPlugin : Plugin<Project> {
    override fun apply(target: Project) = with(target) {
        extensions.findByType(ApplicationExtension::class.java)?.testOptions?.unitTests?.isIncludeAndroidResources = true
        extensions.findByType(LibraryExtension::class.java)?.testOptions?.unitTests?.isIncludeAndroidResources = true

        dependencies {
            add("testImplementation", libs.lib("junit"))
            add("testImplementation", libs.lib("robolectric"))
            add("testImplementation", libs.lib("androidx-test-core"))
            add("testImplementation", libs.lib("androidx-test-ext-junit"))
            add("testImplementation", libs.lib("kotlinx-coroutines-test"))
        }

        tasks.withType<Test>().configureEach {
            maxHeapSize = "2g"
            // Robolectric's SDK 36 runtime reads FileDescriptor internals on JDK 21+.
            jvmArgs("--add-exports=java.base/jdk.internal.access=ALL-UNNAMED", "--add-opens=java.base/java.io=ALL-UNNAMED")
            // Robolectric native graphics, needed for screenshot capture.
            systemProperty("robolectric.pixelCopyRenderMode", "hardware")
            // Optional machine-level override (e.g. a Maven Central mirror) for Robolectric's
            // runtime jar download. Set it in ~/.gradle/gradle.properties, never in the repo.
            providers.gradleProperty("robolectric.dependency.repo.url").orNull?.let {
                systemProperty("robolectric.dependency.repo.url", it)
            }
        }
    }
}
