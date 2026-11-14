package groot.mobile.ui

import android.content.Context
import android.text.format.Formatter
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/** "1.4.0 (142)", or just the name when the code is unknown. */
fun versionLabel(name: String, code: Long): String = if (name.isBlank()) "$code" else "$name ($code)"

fun sizeLabel(context: Context, bytes: Long): String? = bytes.takeIf { it > 0 }?.let { Formatter.formatShortFileSize(context, it) }

/** A hub RFC 3339 timestamp as a local medium date, or null when absent or unparseable. */
fun dateLabel(rfc3339: String, zone: ZoneId = ZoneId.systemDefault()): String? = runCatching {
    OffsetDateTime.parse(rfc3339).atZoneSameInstant(zone).format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
}.getOrNull()

fun dateTimeLabel(epochMillis: Long, zone: ZoneId = ZoneId.systemDefault()): String =
    java.time.Instant.ofEpochMilli(epochMillis).atZone(zone).format(DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT))
