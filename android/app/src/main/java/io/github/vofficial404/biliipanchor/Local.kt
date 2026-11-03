package io.github.vofficial404.biliipanchor

import android.content.Context
import mobile.Mobile
import java.io.File
import java.util.TimeZone

/** 记住的用法（remote 或 local）、服务器地址和代理 */
class Prefs(context: Context) {
    private val sp = context.getSharedPreferences("bia", Context.MODE_PRIVATE)

    var mode: String
        get() = sp.getString("mode", "") ?: ""
        set(v) = sp.edit().putString("mode", v).apply()
    var server: String
        get() = sp.getString("server", "") ?: ""
        set(v) = sp.edit().putString("server", v).apply()
    var proxy: String
        get() = sp.getString("proxy", "") ?: ""
        set(v) = sp.edit().putString("proxy", v).apply()
    var autoLocal: Boolean
        get() = sp.getBoolean("autoLocal", false)
        set(v) = sp.edit().putBoolean("autoLocal", v).apply()
}

/** 本机模式：Go 核心在这个进程里运行（见 mobile/mobile.go），AnchorService 让进程留在后台 */
object Local {
    @Synchronized
    fun start(context: Context, proxy: String) {
        Mobile.setTimeZone(TimeZone.getDefault().id)
        Mobile.start(File(context.filesDir, "data").path, proxy)
    }
}
