package io.github.vofficial404.biliipanchor

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import mobile.Mobile
import org.json.JSONArray
import org.json.JSONException
import org.json.JSONObject

/* 面板接口的数据，字段见 webui/src/lib/api.ts */

data class Zone(val addr: String, val country: String, val province: String, val city: String, val isp: String)

data class Settings(
    val videos: List<String> = emptyList(),
    val doDynamic: Boolean = false,
    val dynamicText: String = "",
    val interval: String = "",
    val jitter: String = "",
    val proxy: String = "",
    val logLevel: String = "info",
) {
    /** PUT /api/settings 的请求体，和网页面板发的一样 */
    fun toJson(): JSONObject = JSONObject()
        .put("videos", JSONArray(videos))
        .put("do_dynamic", doDynamic)
        .put("dynamic_text", dynamicText)
        .put("interval", interval)
        .put("jitter", jitter)
        .put("proxy", proxy)
        .put("log_level", logLevel)
}

data class HomeLocation(val text: String, val checkedAt: Long?, val matchedSince: Long?, val error: String)

data class LastRun(val at: Long?, val ok: Boolean, val error: String)

data class Stats(val coins: Double, val bcoins: Double, val level: Int, val vip: String)

data class FavFailure(val bvid: String, val title: String, val reason: String)

/** 导入收藏的进度，见 core/favorites.go 的 FavImport */
data class FavImport(
    val running: Boolean,
    val folder: String,
    val total: Int,
    val done: Int,
    val added: Int,
    val skipped: Int,
    val failed: Int,
    val failures: List<FavFailure>,
    val created: List<String>,
    val error: String,
    val finished: Long?,
)

data class Account(
    val mid: String,
    val uname: String,
    val face: String,
    val loggedIn: Boolean,
    val running: Boolean,
    val busy: String,
    val last: LastRun?,
    val lastOk: Long?,
    val next: Long?,
    val stats: Stats?,
    val cookieExpires: Long?,
    val canRefresh: Boolean,
    val appAuth: Boolean,
    val home: HomeLocation?,
    val fav: FavImport?,
) {
    val label: String get() = uname.ifEmpty { "UID $mid" }
}

data class ImageInfo(val name: String, val size: Long)

data class Status(
    val version: String,
    val mode: String,
    val accounts: List<Account>,
    val zone: Zone?,
    val zoneError: String,
    val zoneCheckedAt: Long?,
    val settings: Settings,
    val images: List<ImageInfo>,
    val logLevel: String,
)

data class LogEntry(val seq: Long, val time: Long?, val level: String, val mid: String, val account: String, val msg: String)

data class VideoInfo(val bvid: String, val title: String, val cover: String, val owner: String, val duration: Int, val views: Long)

/** 心跳视频的查询结果：info 和 error 有一个 */
data class VideoResult(val info: VideoInfo?, val error: String)

/** Mobile.State() */
data class AppState(
    val mode: String = "",
    val server: String = "",
    val proxy: String = "",
    val remember: Boolean = false,
    val localRunning: Boolean = false,
    val version: String = "",
)

/** 选来导入的收藏夹文件，bytes 原样发给接口 */
class FavFile(
    val target: String,
    val targetLabel: String,
    val bytes: ByteArray,
    val mid: String,
    val uname: String,
    val folders: Int,
    val videos: Int,
    val invalid: Int,
)

/** Go 核心返回的错误（连不上、代理不可用等），message 可以直接显示 */
class CoreException(message: String) : Exception(message)

/** Mobile.call 的结果 */
class Resp(val status: Int, val body: ByteArray, val filename: String, val message: String) {
    val ok: Boolean get() = status in 200..299

    /** 按 JSON 对象解析，不是 JSON 时返回 null */
    fun obj(): JSONObject? = try {
        JSONObject(String(body, Charsets.UTF_8))
    } catch (e: JSONException) {
        null
    }

    fun array(): JSONArray? = try {
        JSONArray(String(body, Charsets.UTF_8))
    } catch (e: JSONException) {
        null
    }
}

/** Go 核心的函数会访问网络或读写文件，全部在 IO 线程上调用；出错时抛 CoreException */
object Api {
    private suspend fun <T> core(block: () -> T): T = withContext(Dispatchers.IO) {
        try {
            block()
        } catch (e: Exception) {
            throw CoreException(e.message?.takeIf { it.isNotBlank() } ?: e.toString())
        }
    }

    suspend fun call(method: String, path: String, body: ByteArray? = null): Resp = core {
        val r = Mobile.call(method, path, body)
        Resp(r.status.toInt(), r.body ?: ByteArray(0), r.filename.orEmpty(), r.message().orEmpty())
    }

    suspend fun callJson(method: String, path: String, body: JSONObject): Resp =
        call(method, path, body.toString().toByteArray(Charsets.UTF_8))

    suspend fun state(): AppState = withContext(Dispatchers.IO) {
        try {
            parseAppState(JSONObject(Mobile.state()))
        } catch (e: Exception) {
            AppState()
        }
    }

    suspend fun probeProxy(proxy: String): Zone = core { parseZone(JSONObject(Mobile.probeProxy(proxy))) }

    /** 开始本机运行，返回代理可能没有生效时的提示 */
    suspend fun startLocal(proxy: String): String = core { JSONObject(Mobile.startLocal(proxy)).str("warning") }

    suspend fun stopLocal() = core { Mobile.stopLocal() }

    suspend fun localRunning(): Boolean = core { Mobile.localRunning() }

    suspend fun connectServer(server: String, password: String, remember: Boolean) =
        core { Mobile.connectServer(server, password, remember) }

    suspend fun disconnect() = core { Mobile.disconnect() }

    /** 下载 B 站图床的图片 */
    suspend fun image(url: String): ByteArray? = core { Mobile.image(url) }
}

/* 解析。Go 的 JSON 里 null 和缺少的字段都当作空值 */

private fun JSONObject.str(name: String): String = if (isNull(name)) "" else optString(name)

private fun JSONObject.time(name: String): Long? = parseTime(str(name))

private fun JSONObject.strings(name: String): List<String> {
    val a = optJSONArray(name) ?: return emptyList()
    return List(a.length()) { a.optString(it) }
}

private fun <T> JSONArray?.objects(f: (JSONObject) -> T): List<T> {
    if (this == null) return emptyList()
    val out = ArrayList<T>(length())
    for (i in 0 until length()) optJSONObject(i)?.let { out.add(f(it)) }
    return out
}

fun parseZone(o: JSONObject) = Zone(o.str("addr"), o.str("country"), o.str("province"), o.str("city"), o.str("isp"))

fun parseSettings(o: JSONObject) = Settings(
    videos = o.strings("videos"),
    doDynamic = o.optBoolean("do_dynamic"),
    dynamicText = o.str("dynamic_text"),
    interval = o.str("interval"),
    jitter = o.str("jitter"),
    proxy = o.str("proxy"),
    logLevel = o.str("log_level").ifEmpty { "info" },
)

private fun parseFav(o: JSONObject) = FavImport(
    running = o.optBoolean("running"),
    folder = o.str("folder"),
    total = o.optInt("total"),
    done = o.optInt("done"),
    added = o.optInt("added"),
    skipped = o.optInt("skipped"),
    failed = o.optInt("failed"),
    failures = o.optJSONArray("failures").objects { FavFailure(it.str("bvid"), it.str("title"), it.str("reason")) },
    created = o.strings("created"),
    error = o.str("error"),
    finished = o.time("finished"),
)

private fun parseAccount(o: JSONObject): Account {
    val cookie = o.optJSONObject("cookie")
    return Account(
        mid = o.str("mid"),
        uname = o.str("uname"),
        face = o.str("face"),
        loggedIn = o.optBoolean("logged_in"),
        running = o.optBoolean("running"),
        busy = o.str("busy"),
        last = o.optJSONObject("last")?.let { LastRun(it.time("at"), it.optBoolean("ok"), it.str("error")) },
        lastOk = o.time("last_ok"),
        next = o.time("next"),
        stats = o.optJSONObject("stats")?.let {
            Stats(it.optDouble("coins", 0.0), it.optDouble("bcoins", 0.0), it.optInt("level"), it.str("vip"))
        },
        cookieExpires = cookie?.time("expires"),
        canRefresh = cookie?.optBoolean("can_refresh") ?: false,
        appAuth = o.optBoolean("app_auth"),
        home = o.optJSONObject("home")?.let {
            HomeLocation(it.str("text"), it.time("checked_at"), it.time("matched_since"), it.str("error"))
        },
        fav = o.optJSONObject("fav_import")?.let(::parseFav),
    )
}

fun parseStatus(o: JSONObject) = Status(
    version = o.str("version"),
    mode = o.str("mode"),
    accounts = o.optJSONArray("accounts").objects(::parseAccount),
    zone = o.optJSONObject("zone")?.let(::parseZone),
    zoneError = o.str("zone_error"),
    zoneCheckedAt = o.time("zone_checked_at"),
    settings = o.optJSONObject("settings")?.let(::parseSettings) ?: Settings(),
    images = o.optJSONArray("images").objects { ImageInfo(it.str("name"), it.optLong("size")) },
    logLevel = o.str("log_level").ifEmpty { "info" },
)

/** GET /api/logs 的 entries */
fun parseLogs(o: JSONObject): List<LogEntry> = o.optJSONArray("entries").objects {
    LogEntry(it.optLong("seq"), it.time("time"), it.str("level"), it.str("mid"), it.str("account"), it.str("msg"))
}

fun parseVideo(o: JSONObject) = VideoInfo(
    o.str("bvid"), o.str("title"), o.str("cover"), o.str("owner"), o.optInt("duration"), o.optLong("views"),
)

fun parseAppState(o: JSONObject) = AppState(
    mode = o.str("mode"),
    server = o.str("server"),
    proxy = o.str("proxy"),
    remember = o.optBoolean("remember"),
    localRunning = o.optBoolean("local_running"),
    version = o.str("version"),
)

private const val BAD_FAV_FILE = "不是 bili-ip-anchor 导出的收藏夹文件"

/** 读出导出文件（core/favorites.go 的 FavExport）的概况，给确认对话框显示；格式不对时抛 IllegalArgumentException */
fun parseFavFile(target: String, targetLabel: String, bytes: ByteArray): FavFile {
    val o = try {
        JSONObject(String(bytes, Charsets.UTF_8))
    } catch (e: JSONException) {
        throw IllegalArgumentException(BAD_FAV_FILE)
    }
    if (o.str("format") != "bili-ip-anchor/favorites" || o.optInt("version") != 1) {
        throw IllegalArgumentException(BAD_FAV_FILE)
    }
    val folders = o.optJSONArray("folders")
    var videos = 0
    var invalid = 0
    for (f in folders.objects { it }) {
        for (v in f.optJSONArray("videos").objects { it }) {
            if (v.optBoolean("invalid")) invalid++ else videos++
        }
    }
    return FavFile(target, targetLabel, bytes, o.str("mid"), o.str("uname"), folders?.length() ?: 0, videos, invalid)
}
