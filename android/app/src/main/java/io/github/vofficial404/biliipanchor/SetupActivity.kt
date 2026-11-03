package io.github.vofficial404.biliipanchor

import android.Manifest
import android.annotation.SuppressLint
import android.app.Activity
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.webkit.JavascriptInterface
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import mobile.Mobile
import org.json.JSONObject

/**
 * 第一页：连接服务器，或者在本机运行。页面是 mobile/shell/setup.html，和 iOS 共用；
 * 它通过 BiaAndroid.call 调用这里。这个桥只挂在这一页的 WebView 上，面板页没有。
 */
class SetupActivity : Activity() {
    private lateinit var web: WebView
    private lateinit var prefs: Prefs

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        prefs = Prefs(this)
        web = WebView(this)
        web.settings.javaScriptEnabled = true
        web.addJavascriptInterface(Bridge(), "BiaAndroid")
        web.webViewClient = object : WebViewClient() {
            // 这一页不跳到别处
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest) = true
        }
        setContentView(web)
        web.loadUrl("file:///android_asset/setup.html")
        if (savedInstanceState == null) autoStart()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        if (Mobile.running()) openLocal() // 点了通知
    }

    override fun onResume() {
        super.onResume()
        web.evaluateJavascript("window.biaRefresh && biaRefresh()", null)
    }

    /** 正在本机运行就直接打开面板；上次勾选了自动开始就开始运行 */
    private fun autoStart() {
        if (Mobile.running()) {
            openLocal()
            return
        }
        val proxy = prefs.proxy
        if (prefs.mode != "local" || !prefs.autoLocal || proxy.isEmpty()) return
        Thread {
            try {
                Local.start(this, proxy)
                runOnUiThread {
                    AnchorService.start(this)
                    openLocal()
                }
            } catch (e: Exception) {
                Log.w(TAG, "自动开始运行失败", e)
            }
        }.start()
    }

    private fun openLocal() {
        startActivity(Intent(this, PanelActivity::class.java).putExtra(PanelActivity.EXTRA_LOCAL, true))
    }

    private fun openRemote(url: String) {
        startActivity(Intent(this, PanelActivity::class.java).putExtra(PanelActivity.EXTRA_URL, url))
    }

    private fun askNotifications() {
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1)
        }
    }

    inner class Bridge {
        /** 在 WebView 的后台线程上调用，返回 JSON；出错时有 error 字段 */
        @JavascriptInterface
        fun call(cmd: String, json: String): String {
            val args = try {
                JSONObject(json)
            } catch (e: Exception) {
                JSONObject()
            }
            val out = JSONObject()
            try {
                when (cmd) {
                    "state" -> {
                        val running = Mobile.running()
                        out.put("mode", prefs.mode)
                        out.put("server", prefs.server)
                        out.put("proxy", if (running) Mobile.proxy() else prefs.proxy)
                        out.put("autoLocal", prefs.autoLocal)
                        out.put("running", running)
                        out.put("version", Mobile.version())
                        out.put("platform", "android")
                    }
                    "probe" -> return Mobile.probeProxy(args.optString("proxy"))
                    "openRemote" -> {
                        val server = args.optString("server")
                        prefs.mode = "remote"
                        prefs.server = server
                        runOnUiThread { openRemote(server) }
                    }
                    "startLocal" -> {
                        val proxy = args.optString("proxy").trim()
                        Local.start(this@SetupActivity, proxy)
                        prefs.mode = "local"
                        prefs.proxy = proxy
                        prefs.autoLocal = args.optBoolean("auto")
                        runOnUiThread {
                            askNotifications()
                            AnchorService.start(this@SetupActivity)
                            openLocal()
                        }
                    }
                    "openLocal" -> runOnUiThread { openLocal() }
                    "stopLocal" -> {
                        AnchorService.stop(this@SetupActivity)
                        Mobile.stop()
                    }
                    else -> out.put("error", "未知命令：$cmd")
                }
            } catch (e: Exception) {
                out.put("error", e.message ?: e.toString())
            }
            return out.toString()
        }
    }

    companion object {
        const val TAG = "BiliIPAnchor"
    }
}
