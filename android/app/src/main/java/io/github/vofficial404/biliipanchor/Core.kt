package io.github.vofficial404.biliipanchor

import android.content.Context
import mobile.Listener
import mobile.Mobile
import java.io.File
import java.util.TimeZone

/**
 * Go 核心（mobile/mobile.go）：在 127.0.0.1 上提供界面（和桌面客户端同一份 MyGo 代码编成的 WebAssembly）
 * 和接口。界面和后台服务都通过这里开始它，同一个进程里只会开始一次。
 */
object Core {
    const val TAG = "BiliIPAnchor"

    /** 开始（已经开始过就不再开始），返回 WebView 要打开的一次性入口地址。会读写文件，不要在主线程上调用。 */
    @Synchronized
    fun start(context: Context): String {
        val app = context.applicationContext
        val web = unpackWeb(app)
        Mobile.setTimeZone(TimeZone.getDefault().id)
        // 本机运行开始、停止时开关前台服务，让进程留在后台
        Mobile.setListener(object : Listener {
            override fun localChanged(running: Boolean) {
                if (running) AnchorService.start(app) else AnchorService.stop(app)
            }
        })
        val url = Mobile.start(web.path, File(app.filesDir, "data").path)
        // 上次本机运行没有被用户停止（进程被回收、手机重启过）：接着运行
        Mobile.resumeLocal()
        return url
    }

    /** 把 APK 里的界面文件（assets/web）解压到 filesDir/web，Go 只能读普通文件。App 更新后重新解压。 */
    private fun unpackWeb(context: Context): File {
        val dir = File(context.filesDir, "web")
        val stamp = File(dir, ".installed")
        @Suppress("DEPRECATION")
        val installed = context.packageManager.getPackageInfo(context.packageName, 0).lastUpdateTime.toString()
        if (stamp.isFile && stamp.readText() == installed) return dir
        dir.deleteRecursively()
        dir.mkdirs()
        val names = context.assets.list("web").orEmpty()
        check(names.contains("index.html")) { "APK 里没有界面文件" }
        for (name in names) {
            context.assets.open("web/$name").use { input ->
                File(dir, name).outputStream().use { input.copyTo(it) }
            }
        }
        stamp.writeText(installed)
        return dir
    }
}
