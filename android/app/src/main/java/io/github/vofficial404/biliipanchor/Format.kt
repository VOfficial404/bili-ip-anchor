package io.github.vofficial404.biliipanchor

import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Locale
import java.util.TimeZone
import kotlin.math.abs
import kotlin.math.roundToLong

/* 显示格式，和 webui/src/lib/format.ts 一样 */

private val TIME_RE = Regex("""^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(\.\d+)?(Z|([+-])(\d{2}):(\d{2}))$""")

/** 解析 Go 的 time.Time（RFC 3339），返回毫秒；空的、零值和解析不了的返回 null。minSdk 24 没有 java.time。 */
fun parseTime(s: String): Long? {
    val m = TIME_RE.matchEntire(s) ?: return null
    if (s.startsWith("0001-")) return null
    return try {
        val f = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss", Locale.US)
        f.timeZone = TimeZone.getTimeZone("UTC")
        var ms = f.parse(m.groupValues[1])?.time ?: return null
        val frac = m.groupValues[2]
        if (frac.length > 1) ms += (frac.substring(1) + "00").substring(0, 3).toLong()
        if (m.groupValues[3] != "Z") {
            val offset = (m.groupValues[5].toLong() * 60 + m.groupValues[6].toLong()) * 60_000
            ms -= if (m.groupValues[4] == "+") offset else -offset
        }
        ms
    } catch (e: Exception) {
        null
    }
}

private fun pad2(n: Int) = n.toString().padStart(2, '0')

private fun cal(ms: Long): Calendar = Calendar.getInstance().apply { timeInMillis = ms }

fun fmtTime(ms: Long): String {
    val c = cal(ms)
    val hm = "${pad2(c.get(Calendar.HOUR_OF_DAY))}:${pad2(c.get(Calendar.MINUTE))}"
    val today = Calendar.getInstance().apply {
        set(Calendar.HOUR_OF_DAY, 0)
        set(Calendar.MINUTE, 0)
        set(Calendar.SECOND, 0)
        set(Calendar.MILLISECOND, 0)
    }
    val day = (ms - today.timeInMillis) / 86_400_000.0
    return when {
        day >= 0 && day < 1 -> "今天 $hm"
        day >= 1 && day < 2 -> "明天 $hm"
        day < 0 && day >= -1 -> "昨天 $hm"
        else -> "${c.get(Calendar.MONTH) + 1}月${c.get(Calendar.DAY_OF_MONTH)}日 $hm"
    }
}

fun fmtDate(ms: Long): String {
    val c = cal(ms)
    return "${c.get(Calendar.YEAR)}-${pad2(c.get(Calendar.MONTH) + 1)}-${pad2(c.get(Calendar.DAY_OF_MONTH))}"
}

/** 日志里的时间：11/03 12:00:05 */
fun fmtLogTime(ms: Long): String {
    val c = cal(ms)
    return "${pad2(c.get(Calendar.MONTH) + 1)}/${pad2(c.get(Calendar.DAY_OF_MONTH))} " +
        "${pad2(c.get(Calendar.HOUR_OF_DAY))}:${pad2(c.get(Calendar.MINUTE))}:${pad2(c.get(Calendar.SECOND))}"
}

fun fmtRelative(ms: Long): String {
    val sec = ((ms - System.currentTimeMillis()) / 1000.0).roundToLong()
    val a = abs(sec)
    val text = when {
        a < 60 -> "不到 1 分钟"
        a < 3600 -> "${(a / 60.0).roundToLong()} 分钟"
        a < 86400 * 2 -> {
            val h = a / 3600
            val m = ((a % 3600) / 60.0).roundToLong()
            if (m > 0) "$h 小时 $m 分钟" else "$h 小时"
        }
        else -> "${a / 86400} 天"
    }
    return if (sec >= 0) "${text}后" else "${text}前"
}

fun fmtDuration(sec: Int): String {
    val m = sec / 60
    return if (m >= 60) "${m / 60}:${pad2(m % 60)}:${pad2(sec % 60)}" else "$m:${pad2(sec % 60)}"
}

fun fmtCount(n: Long): String = when {
    n >= 100_000_000 -> String.format(Locale.US, "%.1f 亿", n / 1e8)
    n >= 10_000 -> String.format(Locale.US, "%.1f 万", n / 1e4)
    else -> n.toString()
}

/** 硬币可能有小数 */
fun fmtNumber(d: Double): String =
    if (d % 1.0 == 0.0) d.toLong().toString() else String.format(Locale.US, "%.1f", d)

/* 属地 */

fun zonePlace(z: Zone): String {
    val parts = if (z.country == "中国") listOf(z.province, z.city) else listOf(z.country, z.province)
    return parts.filter { it.isNotEmpty() }.distinct().joinToString(" ").ifEmpty { "未知地区" }
}

/** 主页属地显示的那一级：国内是省份，国外是国家 */
fun zoneRegion(z: Zone): String = if (z.country == "中国" && z.province.isNotEmpty()) z.province else z.country

fun whereLabel(s: Status): String {
    val who = if (s.mode == "local") "本机" else "这台服务器"
    return if (s.settings.proxy.isNotEmpty()) "$who（经代理）" else who
}

/* 头像和图片 */

private val HDSLB_RE = Regex("""^(https?:)?//[a-z0-9.-]+\.hdslb\.com/""", RegexOption.IGNORE_CASE)
private val SCHEME_RE = Regex("""^(https?:)?//""", RegexOption.IGNORE_CASE)

/** 只接受 B 站图床的图片，加上缩略图参数 */
fun hdslbUrl(src: String, size: String): String {
    if (!HDSLB_RE.containsMatchIn(src)) return ""
    return src.replaceFirst(SCHEME_RE, "https://") + size
}

fun initials(name: String): String {
    val t = name.trim()
    if (t.isEmpty()) return "?"
    val words = t.split(Regex("""\s+"""))
    if (words.size > 1 && t.all { it.code < 128 }) return "${words[0][0]}${words[1][0]}".uppercase()
    return t.substring(0, Character.charCount(t.codePointAt(0)))
}

private val AVATAR_COLORS = longArrayOf(
    0xFFE11D48, 0xFFDB2777, 0xFFC026D3, 0xFF9333EA, 0xFF4F46E5, 0xFF0284C7,
    0xFF0891B2, 0xFF0D9488, 0xFF059669, 0xFF65A30D, 0xFFCA8A04, 0xFFEA580C,
)

/** 头像底色（ARGB），同一个名字总是同一个颜色 */
fun avatarColor(name: String): Long {
    var h = 0L
    var i = 0
    while (i < name.length) {
        val cp = name.codePointAt(i)
        h = (h * 31 + cp) and 0xFFFFFFFFL
        i += Character.charCount(cp)
    }
    return AVATAR_COLORS[(h % AVATAR_COLORS.size).toInt()]
}
