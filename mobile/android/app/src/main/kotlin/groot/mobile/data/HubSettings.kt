package groot.mobile.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.core.stringSetPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import groot.mobile.data.hub.HubEndpoint
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.hubSettingsDataStore: DataStore<Preferences> by preferencesDataStore(name = "hub_settings")

/** How background updates behave. */
data class UpdatePrefs(
    /** Download and install updates in the background (silently where Android allows it). */
    val autoUpdate: Boolean = true,
    /** Only check and download on unmetered networks. */
    val unmeteredOnly: Boolean = true,
    val checkIntervalHours: Int = DEFAULT_INTERVAL_HOURS,
    /** Slugs left out of automatic updates; they still show as updatable. */
    val excludedSlugs: Set<String> = emptySet(),
    /** Epoch millis of the last completed background or manual check; 0 = never. */
    val lastCheckedAt: Long = 0,
) {
    fun autoUpdates(slug: String): Boolean = autoUpdate && slug !in excludedSlugs

    companion object {
        const val DEFAULT_INTERVAL_HOURS = 6
        val INTERVAL_CHOICES = listOf(1, 6, 12, 24)
    }
}

/** The hub connection and update preferences. The token is encrypted with [cipher] before it's stored. */
class HubSettings(private val dataStore: DataStore<Preferences>, private val cipher: SecretCipher) {
    /** The saved hub, or null when not connected (or the token can't be decrypted on this device). */
    val endpoint: Flow<HubEndpoint?> = dataStore.data
        .map { prefs ->
            val url = prefs[BASE_URL] ?: return@map null
            val token = prefs[TOKEN]?.let(cipher::decrypt) ?: return@map null
            HubEndpoint(url, token)
        }
        .distinctUntilChanged()

    /** The saved hub URL, even when the token is unusable, to prefill the connect form. */
    val baseUrl: Flow<String?> = dataStore.data.map { it[BASE_URL] }.distinctUntilChanged()

    val updatePrefs: Flow<UpdatePrefs> = dataStore.data
        .map { prefs ->
            UpdatePrefs(
                autoUpdate = prefs[AUTO_UPDATE] ?: true,
                unmeteredOnly = prefs[UNMETERED_ONLY] ?: true,
                checkIntervalHours = prefs[INTERVAL_HOURS] ?: UpdatePrefs.DEFAULT_INTERVAL_HOURS,
                excludedSlugs = prefs[EXCLUDED] ?: emptySet(),
                lastCheckedAt = prefs[LAST_CHECKED] ?: 0,
            )
        }
        .distinctUntilChanged()

    suspend fun currentEndpoint(): HubEndpoint? = endpoint.first()

    suspend fun currentUpdatePrefs(): UpdatePrefs = updatePrefs.first()

    suspend fun connect(endpoint: HubEndpoint) {
        val encrypted = cipher.encrypt(endpoint.token)
        dataStore.edit {
            it[BASE_URL] = endpoint.baseUrl
            it[TOKEN] = encrypted
        }
    }

    suspend fun disconnect() {
        dataStore.edit {
            it.remove(TOKEN)
            it.remove(LAST_CHECKED)
        }
    }

    suspend fun setAutoUpdate(enabled: Boolean) = dataStore.edit { it[AUTO_UPDATE] = enabled }.let {}

    suspend fun setUnmeteredOnly(enabled: Boolean) = dataStore.edit { it[UNMETERED_ONLY] = enabled }.let {}

    suspend fun setCheckIntervalHours(hours: Int) = dataStore.edit { it[INTERVAL_HOURS] = hours }.let {}

    suspend fun setAutoUpdateFor(slug: String, enabled: Boolean) {
        dataStore.edit {
            val current = it[EXCLUDED] ?: emptySet()
            it[EXCLUDED] = if (enabled) current - slug else current + slug
        }
    }

    suspend fun markChecked(atMillis: Long) = dataStore.edit { it[LAST_CHECKED] = atMillis }.let {}

    suspend fun clear() = dataStore.edit { it.clear() }.let {}

    companion object {
        private val BASE_URL = stringPreferencesKey("base_url")
        private val TOKEN = stringPreferencesKey("token_encrypted")
        private val AUTO_UPDATE = booleanPreferencesKey("auto_update")
        private val UNMETERED_ONLY = booleanPreferencesKey("unmetered_only")
        private val INTERVAL_HOURS = intPreferencesKey("check_interval_hours")
        private val EXCLUDED = stringSetPreferencesKey("auto_update_excluded")
        private val LAST_CHECKED = longPreferencesKey("last_checked_at")

        fun create(context: Context, cipher: SecretCipher = KeystoreSecretCipher()) =
            HubSettings(context.applicationContext.hubSettingsDataStore, cipher)
    }
}
