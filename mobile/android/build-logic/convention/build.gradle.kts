plugins {
    `kotlin-dsl`
}

group = "groot.buildlogic"

dependencies {
    compileOnly(libs.android.gradlePlugin)
    compileOnly(libs.kotlin.gradlePlugin)
    compileOnly(libs.compose.gradlePlugin)
}

gradlePlugin {
    plugins {
        register("androidApplication") {
            id = "groot.android.application"
            implementationClass = "AndroidApplicationConventionPlugin"
        }
        register("androidLibrary") {
            id = "groot.android.library"
            implementationClass = "AndroidLibraryConventionPlugin"
        }
        register("compose") {
            id = "groot.compose"
            implementationClass = "ComposeConventionPlugin"
        }
        register("test") {
            id = "groot.test"
            implementationClass = "TestConventionPlugin"
        }
    }
}
