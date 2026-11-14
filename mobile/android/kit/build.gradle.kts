// KIT (synced, read-only in children). Explicit API mode is on via groot.android.library.
plugins {
    alias(libs.plugins.groot.android.library)
    alias(libs.plugins.groot.compose)
    alias(libs.plugins.groot.test)
}

android {
    namespace = "groot.kit"
}

dependencies {
    implementation(libs.androidx.core)
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.foundation)
    implementation(libs.androidx.compose.material3)
    // api: ThemeModeStore's public constructor takes a DataStore.
    api(libs.androidx.datastore.preferences)
    implementation(libs.androidx.profileinstaller)
}
