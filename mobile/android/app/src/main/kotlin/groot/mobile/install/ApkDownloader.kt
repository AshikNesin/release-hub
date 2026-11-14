package groot.mobile.install

import groot.mobile.data.hub.HubManifest
import java.io.File
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext

/** Downloads an APK and proves it's the one the hub described. */
interface ApkDownloader {
    /** Writes the APK to [dest]; throws [InstallException] on network failure or a size/checksum mismatch. */
    suspend fun download(manifest: HubManifest, dest: File, onProgress: (Float?) -> Unit)
}

class InstallException(message: String, cause: Throwable? = null) : Exception(message, cause)

/**
 * [ApkDownloader] over HttpURLConnection. Artifact URLs are public (hub, S3 presigned or CDN), so the hub
 * token is never sent with them. The SHA-256 from the manifest is checked before anything is installed.
 */
class HttpApkDownloader : ApkDownloader {
    override suspend fun download(manifest: HubManifest, dest: File, onProgress: (Float?) -> Unit) =
        withContext(Dispatchers.IO) {
            dest.parentFile?.mkdirs()
            val partial = File(dest.path + ".part")
            val digest = MessageDigest.getInstance("SHA-256")
            val connection = try {
                URL(manifest.apkUrl).openConnection() as HttpURLConnection
            } catch (e: IOException) {
                throw InstallException("Can't open the download link", e)
            }
            try {
                connection.connectTimeout = TIMEOUT_MS
                connection.readTimeout = TIMEOUT_MS
                val status = connection.responseCode
                if (status !in 200..299) throw InstallException("Download failed (HTTP $status)")
                val total = connection.contentLengthLong.takeIf { it > 0 } ?: manifest.size.takeIf { it > 0 }
                var read = 0L
                var lastReported = -1
                connection.inputStream.use { input ->
                    partial.outputStream().use { output ->
                        val buffer = ByteArray(BUFFER_BYTES)
                        while (true) {
                            ensureActive()
                            val n = input.read(buffer)
                            if (n < 0) break
                            output.write(buffer, 0, n)
                            digest.update(buffer, 0, n)
                            read += n
                            val percent = if (total != null) (read * 100 / total).toInt() else -1
                            if (percent != lastReported) {
                                lastReported = percent
                                onProgress(if (total != null) (read.toFloat() / total).coerceIn(0f, 1f) else null)
                            }
                        }
                    }
                }
                verify(manifest, read, digest.digest().toHex())
                if (!partial.renameTo(dest)) throw InstallException("Couldn't save the download")
            } catch (e: IOException) {
                throw InstallException("Download interrupted: ${e.message ?: e.javaClass.simpleName}", e)
            } finally {
                connection.disconnect()
                partial.delete()
            }
        }

    private companion object {
        const val TIMEOUT_MS = 30_000
        const val BUFFER_BYTES = 64 * 1024
    }
}

/** Throws when the bytes aren't the release the hub described. Size and hash are skipped when the hub omits them. */
internal fun verify(manifest: HubManifest, size: Long, sha256: String) {
    if (manifest.size > 0 && size != manifest.size) {
        throw InstallException("Download size doesn't match the release ($size of ${manifest.size} bytes)")
    }
    if (manifest.sha256.isNotEmpty() && !manifest.sha256.equals(sha256, ignoreCase = true)) {
        throw InstallException("Checksum mismatch: the download isn't the release the hub published")
    }
}

internal fun ByteArray.toHex(): String = joinToString("") { "%02x".format(it) }
