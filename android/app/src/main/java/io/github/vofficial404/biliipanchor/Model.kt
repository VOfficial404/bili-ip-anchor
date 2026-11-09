package io.github.vofficial404.biliipanchor

import android.app.Application
import android.net.Uri
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.IOException

enum class Phase { Loading, Failed, Setup, Main }

/** 测试代理的结果；ok 为 null 是正在测试或者不需要测试 */
data class Probe(val text: String, val ok: Boolean?)

/** 扫码：mid 为 null 是添加账号，否则是授权查询这个账号的主页属地 */
data class QrTarget(val mid: String?, val label: String)

private const val MAX_IMAGE = 10 shl 20 // 和 core 的 maxImageSize 一样
private const val MAX_FAV_FILE = 20 shl 20 // 和 handleFavImport 的上限一样
private const val MAX_LOGS = 1000 // 和服务端日志缓冲一样多
private val BV_RE = Regex("BV[0-9A-Za-z]{10}")

fun zoneText(z: Zone): String {
    val isp = if (z.isp.isNotEmpty()) " · ${z.isp}" else ""
    return "B 站看到的是 ${zonePlace(z)}$isp · ${z.addr}，主页属地会变成${zoneRegion(z)}。"
}

/** 整个 App 的状态。Go 核心的调用都在 IO 线程上，状态只在主线程上改。 */
class AppViewModel(app: Application) : AndroidViewModel(app) {
    var phase by mutableStateOf(Phase.Loading)
        private set
    var startError by mutableStateOf("")
        private set
    var appState by mutableStateOf(AppState())
        private set

    /** 显示成对话框的提示（例如代理可能没有生效） */
    var notice by mutableStateOf<String?>(null)

    private val messages = Channel<String>(Channel.UNLIMITED)

    /** 显示成 Snackbar 的提示 */
    val messageFlow: Flow<String> = messages.receiveAsFlow()

    fun toast(msg: String) {
        if (msg.isNotEmpty()) messages.trySend(msg)
    }

    /* 首页 */

    var setupBusy by mutableStateOf(false)
        private set
    var setupProbing by mutableStateOf(false)
        private set
    var setupError by mutableStateOf("")
    var setupProbe by mutableStateOf<Probe?>(null)

    /* 主界面 */

    var status by mutableStateOf<Status?>(null)
        private set
    var statusError by mutableStateOf("")
        private set
    var refreshing by mutableStateOf(false)
        private set
    var needPassword by mutableStateOf(false)
        private set
    var passwordError by mutableStateOf("")
        private set
    var reconnecting by mutableStateOf(false)
        private set

    /** 本机运行已经停止（用户停的，或者通知里的「停止」） */
    var stopped by mutableStateOf(false)
        private set
    var restarting by mutableStateOf(false)
        private set
    var runningAll by mutableStateOf(false)
        private set

    /** 正在操作的账号 */
    var pending by mutableStateOf(emptySet<String>())
        private set

    /** 正在导出收藏夹的账号 */
    var exporting by mutableStateOf("")
        private set
    var favConfirm by mutableStateOf<FavFile?>(null)
    private var exportBytes: ByteArray? = null
    private var importTarget: Account? = null

    /* 设置 */

    var form by mutableStateOf(Settings())
        private set
    var edited by mutableStateOf(false)
        private set
    var saving by mutableStateOf(false)
        private set
    var formError by mutableStateOf("")
        private set
    var proxyProbe by mutableStateOf<Probe?>(null)
    var testingProxy by mutableStateOf(false)
        private set
    var videoError by mutableStateOf("")
    var addingVideos by mutableStateOf(false)
        private set
    var uploading by mutableStateOf(false)
        private set
    var deletingImage by mutableStateOf("")
        private set
    val videos = mutableStateMapOf<String, VideoResult>()
    private val videoLoading = HashSet<String>()

    /* 日志 */

    var logs by mutableStateOf<List<LogEntry>>(emptyList())
        private set
    var logError by mutableStateOf("")
        private set
    private var lastSeq = 0L

    init {
        start()
    }

    /** 打开 Go 核心，没选过用法时显示首页 */
    fun start() = viewModelScope.launch {
        phase = Phase.Loading
        try {
            withContext(Dispatchers.IO) { Core.start(getApplication<Application>()) }
        } catch (e: Exception) {
            startError = e.message ?: e.toString()
            phase = Phase.Failed
            return@launch
        }
        val st = Api.state()
        if (st.mode.isEmpty()) {
            appState = st
            phase = Phase.Setup
        } else {
            enterMain()
        }
    }

    private fun resetMain() {
        status = null
        statusError = ""
        needPassword = false
        passwordError = ""
        pending = emptySet()
        exporting = ""
        favConfirm = null
        form = Settings()
        edited = false
        formError = ""
        proxyProbe = null
        videoError = ""
        videos.clear()
        logs = emptyList()
        logError = ""
        lastSeq = 0
    }

    private suspend fun enterMain() {
        resetMain()
        appState = Api.state()
        stopped = appState.mode == "local" && !appState.localRunning
        phase = Phase.Main
    }

    /* 首页的操作 */

    fun probeSetup(proxy: String) = viewModelScope.launch {
        if (proxy.isBlank()) {
            setupProbe = Probe("请填写代理的地址", false)
            return@launch
        }
        setupProbing = true
        setupProbe = Probe("正在经过代理访问 B 站…", null)
        setupProbe = try {
            Probe("可用。" + zoneText(Api.probeProxy(proxy.trim())), true)
        } catch (e: CoreException) {
            Probe(e.message.orEmpty(), false)
        }
        setupProbing = false
    }

    fun startLocal(proxy: String) = viewModelScope.launch {
        setupBusy = true
        setupError = ""
        try {
            val warning = Api.startLocal(proxy)
            if (warning.isNotEmpty()) notice = warning
            enterMain()
        } catch (e: CoreException) {
            setupError = e.message.orEmpty()
        }
        setupBusy = false
    }

    fun connect(server: String, password: String, remember: Boolean) = viewModelScope.launch {
        setupBusy = true
        setupError = ""
        try {
            Api.connectServer(server, password, remember)
            enterMain()
        } catch (e: CoreException) {
            setupError = e.message.orEmpty()
        }
        setupBusy = false
    }

    /* 接口 */

    /**
     * 调用面板接口。连不上时返回状态码为 0 的 Resp。
     * 服务器要求重新输入密码（401）时弹出密码框；本机运行已经停止（409）时显示横幅。
     */
    suspend fun call(method: String, path: String, body: ByteArray? = null): Resp {
        val r = try {
            Api.call(method, path, body)
        } catch (e: CoreException) {
            return Resp(0, ByteArray(0), "", e.message.orEmpty())
        }
        if (r.status == 401 && appState.mode == "remote") {
            needPassword = true
        } else if (r.status == 409 && appState.mode == "local" && !localRunning()) {
            stopped = true
        }
        return r
    }

    private suspend fun localRunning(): Boolean = try {
        Api.localRunning()
    } catch (e: CoreException) {
        false
    }

    /** 调用接口，出错时提示；成功时返回结果，有 done 时提示它 */
    suspend fun act(method: String, path: String, body: ByteArray? = null, done: String? = null): Resp? {
        val r = call(method, path, body)
        if (r.ok) {
            if (done != null) toast(done)
            return r
        }
        if (!(r.status == 401 && needPassword)) toast(r.message)
        return null
    }

    suspend fun refreshStatus() {
        if (phase != Phase.Main || needPassword) return
        val r = call("GET", "/api/status")
        val o = if (r.ok) r.obj() else null
        if (o != null) {
            val s = parseStatus(o)
            status = s
            statusError = ""
            stopped = false
            // 没改过时跟着服务端的设置走（别的客户端改了也能看到）
            if (!edited) form = s.settings
        } else if (r.ok) {
            statusError = "状态不是 JSON"
        } else if (!(r.status == 401 && needPassword) && !stopped) {
            statusError = r.message
        }
    }

    fun refresh() = viewModelScope.launch { refreshStatus() }

    fun pullRefresh() = viewModelScope.launch {
        refreshing = true
        refreshStatus()
        refreshing = false
    }

    /* 连接 */

    fun reconnect(password: String, remember: Boolean) = viewModelScope.launch {
        reconnecting = true
        passwordError = ""
        try {
            Api.connectServer(appState.server, password, remember)
            appState = Api.state()
            needPassword = false
            refreshStatus()
        } catch (e: CoreException) {
            passwordError = e.message.orEmpty()
        }
        reconnecting = false
    }

    /** 回到首页：停下本机运行，或者退出服务器的登录 */
    fun disconnect() = viewModelScope.launch {
        try {
            Api.disconnect()
        } catch (e: CoreException) {
            toast(e.message.orEmpty())
        }
        resetMain()
        stopped = false
        appState = Api.state()
        setupError = ""
        setupProbe = null
        phase = Phase.Setup
    }

    fun stopLocal() = viewModelScope.launch {
        try {
            Api.stopLocal()
        } catch (e: CoreException) {
            toast(e.message.orEmpty())
        }
        appState = Api.state()
        stopped = true
        toast("已停止本机运行")
    }

    /** 用保存的代理重新开始本机运行 */
    fun restartLocal() = viewModelScope.launch {
        val proxy = status?.settings?.proxy?.takeIf { it.isNotBlank() } ?: appState.proxy
        restarting = true
        try {
            val warning = Api.startLocal(proxy)
            if (warning.isNotEmpty()) notice = warning
            appState = Api.state()
            stopped = false
            // 新的定时任务的日志序号从头开始
            logs = emptyList()
            lastSeq = 0
            refreshStatus()
        } catch (e: CoreException) {
            toast(e.message.orEmpty())
        }
        restarting = false
    }

    /* 账号 */

    fun runAll() = viewModelScope.launch {
        runningAll = true
        val r = act("POST", "/api/run")
        if (r != null) toast("已让 ${r.obj()?.optInt("triggered") ?: 0} 个账号立即运行")
        runningAll = false
        delay(600)
        refreshStatus()
    }

    /** POST/DELETE /api/accounts/{mid}/{action}；action 为空时是账号本身 */
    fun accountAction(a: Account, method: String, action: String, done: String?) = viewModelScope.launch {
        pending = pending + a.mid
        val path = "/api/accounts/${a.mid}" + if (action.isEmpty()) "" else "/$action"
        act(method, path, done = done)
        pending = pending - a.mid
        refreshStatus()
    }

    fun qrDone(target: QrTarget, mid: String) = viewModelScope.launch {
        if (target.mid == null) toast("已添加账号 UID $mid，正在运行第一轮")
        else toast("已授权，正在查询「${target.label}」的主页属地")
        refreshStatus()
        delay(3000)
        refreshStatus()
    }

    /** 导出收藏夹：读完后调用 save 让用户选保存的位置 */
    fun exportFavorites(a: Account, save: (String) -> Unit) = viewModelScope.launch {
        exporting = a.mid
        val r = act("GET", "/api/accounts/${a.mid}/favorites")
        exporting = ""
        if (r != null) {
            exportBytes = r.body
            save(r.filename.ifEmpty { "bili-favorites-${a.mid}.json" })
        }
    }

    fun saveExport(uri: Uri?) {
        val bytes = exportBytes
        exportBytes = null
        if (uri == null || bytes == null) return
        viewModelScope.launch {
            try {
                withContext(Dispatchers.IO) {
                    val out = getApplication<Application>().contentResolver.openOutputStream(uri)
                        ?: throw IOException("打不开这个文件")
                    out.use { it.write(bytes) }
                }
                toast("收藏夹已导出")
            } catch (e: Exception) {
                toast("保存失败：${e.message ?: e}")
            }
        }
    }

    fun beginImport(a: Account) {
        importTarget = a
    }

    /** 选好了要导入的文件：读出来，显示确认对话框 */
    fun pickedImport(uri: Uri?) {
        val target = importTarget
        importTarget = null
        if (uri == null || target == null) return
        viewModelScope.launch {
            try {
                val bytes = withContext(Dispatchers.IO) { readUri(uri, MAX_FAV_FILE, "文件太大，不像是导出的收藏夹") }
                favConfirm = withContext(Dispatchers.Default) { parseFavFile(target.mid, target.label, bytes) }
            } catch (e: IllegalArgumentException) {
                toast(e.message.orEmpty())
            } catch (e: IOException) {
                toast("读不了这个文件：${e.message ?: e}")
            }
        }
    }

    fun confirmImport(f: FavFile) {
        favConfirm = null
        viewModelScope.launch {
            val r = act("POST", "/api/accounts/${f.target}/favorites", f.bytes)
            if (r != null) {
                val total = r.obj()?.optInt("total") ?: 0
                toast("开始导入 $total 个视频，进度显示在账号卡片上")
            }
            refreshStatus()
        }
    }

    private fun readUri(uri: Uri, limit: Int, tooBig: String): ByteArray {
        val input = getApplication<Application>().contentResolver.openInputStream(uri)
            ?: throw IOException("打不开这个文件")
        input.use { s ->
            val out = ByteArrayOutputStream()
            val buf = ByteArray(64 * 1024)
            var total = 0
            while (true) {
                val n = s.read(buf)
                if (n < 0) break
                total += n
                if (total > limit) throw IOException(tooBig)
                out.write(buf, 0, n)
            }
            return out.toByteArray()
        }
    }

    /* 设置 */

    fun updateForm(f: Settings) {
        form = f
        edited = true
    }

    fun resetForm() {
        status?.let { form = it.settings }
        edited = false
        formError = ""
        videoError = ""
        proxyProbe = null
    }

    fun saveSettings() = viewModelScope.launch {
        saving = true
        formError = ""
        val f = form
        val body = f.copy(
            interval = f.interval.trim(),
            jitter = f.jitter.trim(),
            proxy = f.proxy.trim(),
            dynamicText = f.dynamicText.trim(),
        ).toJson()
        val r = call("PUT", "/api/settings", body.toString().toByteArray(Charsets.UTF_8))
        if (r.ok) {
            r.obj()?.let { form = parseSettings(it) }
            edited = false
            toast("设置已保存")
            refreshStatus()
        } else if (!(r.status == 401 && needPassword)) {
            formError = r.message
        }
        saving = false
    }

    fun testProxy() = viewModelScope.launch {
        val proxy = form.proxy.trim()
        if (proxy.isEmpty()) {
            proxyProbe = Probe("留空表示直连，不需要测试。", null)
            return@launch
        }
        testingProxy = true
        proxyProbe = Probe("正在经过代理访问 B 站…", null)
        val body = JSONObject().put("proxy", proxy).toString().toByteArray(Charsets.UTF_8)
        val r = call("POST", "/api/proxy/test", body)
        val o = if (r.ok) r.obj() else null
        proxyProbe = when {
            o != null -> Probe("可用。" + zoneText(parseZone(o)), true)
            r.status == 401 && needPassword -> null
            else -> Probe(r.message.ifEmpty { "测试失败" }, false)
        }
        testingProxy = false
    }

    /** 查询心跳视频的标题和封面，结果放在 videos 里 */
    fun loadVideo(bvid: String) {
        if (videos.containsKey(bvid) || !videoLoading.add(bvid)) return
        viewModelScope.launch {
            val r = call("GET", "/api/videos/$bvid")
            videoLoading.remove(bvid)
            val o = if (r.ok) r.obj() else null
            if (o != null) {
                videos.put(bvid, VideoResult(parseVideo(o), ""))
            } else if (r.status != 401 && r.status != 409) {
                videos.put(bvid, VideoResult(null, r.message))
            }
        }
    }

    /** 从粘贴的文字里找出 BV 号，查询确认视频存在后加进表单 */
    fun addVideos(text: String, added: () -> Unit) = viewModelScope.launch {
        val found = BV_RE.findAll(text).map { it.value }.distinct().toList()
        if (found.isEmpty()) {
            videoError = "没找到 BV 号。粘贴视频链接或 BV 开头的 12 位编号"
            return@launch
        }
        addingVideos = true
        videoError = ""
        val bad = mutableListOf<String>()
        val ok = mutableListOf<String>()
        for (bvid in found) {
            if (bvid in form.videos || bvid in ok) continue
            if (form.videos.size + ok.size >= 20) {
                bad.add("最多 20 个视频")
                break
            }
            val r = call("GET", "/api/videos/$bvid")
            if (r.ok) {
                r.obj()?.let { videos[bvid] = VideoResult(parseVideo(it), "") }
                ok.add(bvid)
            } else if (r.status == 401) {
                break
            } else {
                bad.add("$bvid：${r.message}")
            }
        }
        addingVideos = false
        added()
        if (ok.isNotEmpty()) updateForm(form.copy(videos = form.videos + ok))
        if (bad.isNotEmpty()) videoError = bad.joinToString("；")
    }

    /* 动态图片：上传和删除立即生效，不需要保存 */

    fun uploadImages(uris: List<Uri>) {
        if (uris.isEmpty()) return
        viewModelScope.launch {
            uploading = true
            var n = 0
            for (uri in uris) {
                try {
                    val bytes = withContext(Dispatchers.IO) { readUri(uri, MAX_IMAGE, "图片不能超过 10 MB") }
                    if (act("POST", "/api/dynamic/images", bytes) != null) n++ else break
                } catch (e: IOException) {
                    toast(e.message ?: "读不了这张图片")
                }
            }
            uploading = false
            if (n > 0) toast("已添加 $n 张图片")
            refreshStatus()
        }
    }

    fun deleteImage(name: String) = viewModelScope.launch {
        deletingImage = name
        act("DELETE", "/api/dynamic/images/$name")
        Bitmaps.remove(imageKey(name))
        refreshStatus()
        deletingImage = ""
    }

    fun imageKey(name: String) = "dynamic:$name"

    /** 动态图片的原始数据，给缩略图用 */
    suspend fun imageBytes(name: String): ByteArray? {
        val r = try {
            Api.call("GET", "/api/dynamic/images/$name")
        } catch (e: CoreException) {
            return null
        }
        return if (r.ok) r.body else null
    }

    /* 日志：第一次拉全部，之后只拉序号更大的 */

    suspend fun pollLogs() {
        if (phase != Phase.Main || needPassword) return
        val r = call("GET", "/api/logs?after=$lastSeq")
        if (!r.ok) {
            if (!(r.status == 401 && needPassword) && !stopped) logError = r.message
            return
        }
        logError = ""
        val list = r.obj()?.let(::parseLogs).orEmpty().filter { it.seq > lastSeq }
        if (list.isNotEmpty()) {
            lastSeq = list.last().seq
            logs = (logs + list).takeLast(MAX_LOGS)
        }
    }
}
