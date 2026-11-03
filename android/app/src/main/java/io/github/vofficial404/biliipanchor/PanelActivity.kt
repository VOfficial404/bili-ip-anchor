package io.github.vofficial404.biliipanchor

import android.annotation.SuppressLint
import android.app.Activity
import android.app.AlertDialog
import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.webkit.CookieManager
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import mobile.Mobile

/** 面板：在 WebView 里打开 WebUI。本机模式打开自己开在 127.0.0.1 上的，否则打开服务器的。 */
class PanelActivity : Activity() {
    private lateinit var web: WebView
    private var local = false
    private var origin = ""
    private var fileCallback: ValueCallback<Array<Uri>>? = null

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        local = intent.getBooleanExtra(EXTRA_LOCAL, false)
        val url = if (local) {
            try {
                Mobile.loginURL()
            } catch (e: Exception) {
                finish() // 进程被系统回收过，核心没在跑
                return
            }
        } else {
            intent.getStringExtra(EXTRA_URL)
        }
        if (url.isNullOrEmpty()) {
            finish()
            return
        }
        origin = originOf(Uri.parse(url))

        web = WebView(this)
        web.settings.javaScriptEnabled = true
        web.settings.domStorageEnabled = true
        CookieManager.getInstance().setAcceptCookie(true)
        web.webViewClient = object : WebViewClient() {
            // 面板以外的链接（B 站视频、GitHub）用浏览器打开
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                if (originOf(request.url) == origin) return false
                try {
                    startActivity(Intent(Intent.ACTION_VIEW, request.url))
                } catch (e: ActivityNotFoundException) {
                }
                return true
            }
        }
        web.webChromeClient = object : WebChromeClient() {
            // 设置页「添加图片」
            override fun onShowFileChooser(view: WebView, callback: ValueCallback<Array<Uri>>, params: FileChooserParams): Boolean {
                fileCallback?.onReceiveValue(null)
                fileCallback = callback
                val pick = params.createIntent()
                if (params.mode == FileChooserParams.MODE_OPEN_MULTIPLE) pick.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true)
                return try {
                    startActivityForResult(pick, PICK_FILES)
                    true
                } catch (e: ActivityNotFoundException) {
                    fileCallback = null
                    false
                }
            }
        }
        setContentView(web)
        web.loadUrl(url)
    }

    @Deprecated("Activity 的回调")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != PICK_FILES) return
        var uris: Array<Uri>? = null
        if (resultCode == RESULT_OK && data != null) {
            val clip = data.clipData
            uris = if (clip != null) Array(clip.itemCount) { clip.getItemAt(it).uri } else data.data?.let { arrayOf(it) }
        }
        fileCallback?.onReceiveValue(uris)
        fileCallback = null
    }

    override fun onResume() {
        super.onResume()
        if (local && !Mobile.running()) finish()
    }

    @Deprecated("Activity 的回调")
    override fun onBackPressed() {
        if (::web.isInitialized && web.canGoBack()) {
            web.goBack()
            return
        }
        val items = if (local) {
            arrayOf("返回首页", "停止本机运行", "退到后台（继续运行）")
        } else {
            arrayOf("返回首页，换一台服务器", "退出")
        }
        AlertDialog.Builder(this).setItems(items) { _, which ->
            when {
                which == 0 -> finish()
                local && which == 1 -> {
                    AnchorService.stop(this)
                    Mobile.stop()
                    finish()
                }
                local -> moveTaskToBack(true)
                else -> finishAffinity()
            }
        }.show()
    }

    override fun onDestroy() {
        if (::web.isInitialized) web.destroy()
        super.onDestroy()
    }

    companion object {
        const val EXTRA_LOCAL = "local"
        const val EXTRA_URL = "url"
        private const val PICK_FILES = 2

        fun originOf(u: Uri) = "${u.scheme}://${u.encodedAuthority}"
    }
}
