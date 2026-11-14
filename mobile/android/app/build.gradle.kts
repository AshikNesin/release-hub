// APP-owned: your app's build file. Shared config comes from the groot.* convention plugins.
import java.util.Properties

plugins {
    alias(libs.plugins.groot.android.application)
    alias(libs.plugins.groot.compose)
    alias(libs.plugins.groot.test)
    alias(libs.plugins.roborazzi)
}

// Versions live in version.properties, never in this file (AGENTS.md).
val versionProps = Properties().apply {
    rootProject.file("version.properties").inputStream().use { load(it) }
}

val gitCommit: String = providers.exec {
    commandLine("git", "rev-parse", "--short", "HEAD")
    isIgnoreExitValue = true
}.standardOutput.asText.map { it.trim().ifEmpty { "unknown" } }.getOrElse("unknown")

android {
    // The Kotlin namespace stays fixed; groot:setup changes only applicationId.
    namespace = "groot.mobile"

    defaultConfig {
        applicationId = "io.nesin.releasehub"
        versionName = versionProps.getProperty("VERSION_NAME")
        versionCode = versionProps.getProperty("VERSION_CODE").toInt()
        buildConfigField("String", "GIT_COMMIT", "\"$gitCommit\"")
    }

    buildFeatures {
        buildConfig = true
    }
}

roborazzi {
    outputDir.set(file("build/outputs/roborazzi"))
}

dependencies {
    implementation(project(":kit"))
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.navigation.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.compose.material3)

    testImplementation(libs.roborazzi)
    testImplementation(libs.roborazzi.compose)
    testImplementation(libs.roborazzi.junit.rule)
}
