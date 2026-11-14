package groot.mobile.data.hub

import org.json.JSONArray
import org.json.JSONException
import org.json.JSONObject

/** Parsers for the hub's bearer and public API. Pure functions over response bodies, so tests need no server. */
object HubJson {
    /** Apps with an android platform; iOS-only apps are left out. */
    fun parseApps(body: String): List<HubApp> = parse(body) {
        val array = JSONArray(body)
        buildList {
            for (i in 0 until array.length()) {
                val app = array.getJSONObject(i)
                val platforms = app.optJSONArray("platforms") ?: continue
                for (j in 0 until platforms.length()) {
                    val p = platforms.getJSONObject(j)
                    if (p.optString("platform") == "android") {
                        add(HubApp(slug = app.getString("slug"), packageName = p.getString("packageName")))
                    }
                }
            }
        }.sortedBy { it.slug }
    }

    fun parseManifest(body: String): HubManifest = parse(body) {
        val o = JSONObject(body)
        HubManifest(
            versionCode = o.getLong("versionCode"),
            versionName = o.optString("versionName"),
            apkUrl = o.getString("apkUrl"),
            sha256 = o.optString("sha256"),
            size = o.optLong("size"),
            notes = o.optString("notes"),
            createdAt = o.optString("createdAt"),
        )
    }

    /** Newest first, as the hub returns them. */
    fun parseReleases(body: String): List<HubRelease> = parse(body) {
        val array = JSONArray(body)
        List(array.length()) { i ->
            val o: JSONObject = array.getJSONObject(i)
            HubRelease(
                versionCode = o.getLong("versionCode"),
                versionName = o.optString("versionName"),
                channel = o.optString("channel"),
                url = o.optString("url"),
                sha256 = o.optString("sha256"),
                size = o.optLong("size"),
                createdAt = o.optString("createdAt"),
                fileName = o.optString("fileName"),
                notes = o.optString("notes"),
            )
        }.sortedByDescending { it.versionCode }
    }

    private inline fun <T> parse(body: String, block: () -> T): T = try {
        block()
    } catch (e: JSONException) {
        throw HubException("The hub sent a response this app doesn't understand", cause = e)
    }
}
