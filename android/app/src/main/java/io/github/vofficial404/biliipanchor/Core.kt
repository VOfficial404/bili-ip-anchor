package io.github.vofficial404.biliipanchor

import android.content.Context
import android.util.Log
import mobile.Listener
import mobile.Mobile
import java.io.File
import java.util.TimeZone

/**
 * Go 核心（mobile/mobile.go）：管本机运行的定时任务和服务器的连接，界面通过 Mobile.call 调用面板接口。
 * 界面和后台服务都通过这里打开它；重复调用没有关系，Open 只会打开一次。
 */
object Core {
    const val TAG = "BiliIPAnchor"

    /** 打开核心；上次本机运行没有被用户停止时接着运行。会读写文件，不要在主线程上调用。 */
    @Synchronized
    fun start(context: Context) {
        val app = context.applicationContext
        Mobile.setTimeZone(TimeZone.getDefault().id)
        // 本机运行开始、停止时开关前台服务，让进程留在后台
        Mobile.setListener(object : Listener {
            override fun localChanged(running: Boolean) {
                if (running) AnchorService.start(app) else AnchorService.stop(app)
            }
        })
        Mobile.open(File(app.filesDir, "data").path)
        // 上次本机运行没有被用户停止（进程被回收、手机重启过）：接着运行
        try {
            Mobile.resumeLocal()
        } catch (e: Exception) {
            Log.w(TAG, "接着本机运行失败", e)
        }
    }
}
