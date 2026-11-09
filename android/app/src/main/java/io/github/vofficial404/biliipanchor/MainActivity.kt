package io.github.vofficial404.biliipanchor

import android.Manifest
import android.annotation.SuppressLint
import android.app.Activity
import android.app.AlertDialog
import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Message
import android.util.Log
import android.view.ViewGroup
import android.webkit.RenderProcessGoneDetail
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient

/** 唯一的页面：WebView 打开 Go 核心在 127.0.0.1 上提供的界面。连接服务器、本机运行都在界面里选。 */
class MainActivity : Activity() {
    private lateinit var web: WebView
    private var origin = ""
    private var fileCallback: ValueCallback<Array<Uri>>? = null

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        askNotifications()

        web = WebView(this)
        web.settings.javaScriptEnabled = true
        web.settings.domStorageEnabled = true
        web.settings.setSupportZoom(false)
        // 界面用 window.open 打开 B 站视频等链接，交给 onCreateWindow 转到浏览器
        web.settings.setSupportMultipleWindows(true)
        web.settings.javaScriptCanOpenWindowsAutomatically = true
        web.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                if (originOf(request.url) == origin) return false
                openOutside(request.url)
                return true
            }

            // WebView 的渲染进程崩溃或被回收：重新创建页面，界面会重新连接
            override fun onRenderProcessGone(view: WebView, detail: RenderProcessGoneDetail): Boolean {
                Log.w(Core.TAG, "WebView 渲染进程退出，重新打开")
                (view.parent as? ViewGroup)?.removeView(view)
                view.destroy()
                recreate()
                return true
            }
        }
        web.webChromeClient = object : WebChromeClient() {
            override fun onCreateWindow(view: WebView, isDialog: Boolean, isUserGesture: Boolean, resultMsg: Message): Boolean {
                val tmp = WebView(this@MainActivity)
                tmp.webViewClient = object : WebViewClient() {
                    override fun shouldOverrideUrlLoading(v: WebView, request: WebResourceRequest): Boolean {
                        openOutside(request.url)
                        v.destroy()
                        return true
                    }
                }
                (resultMsg.obj as WebView.WebViewTransport).webView = tmp
                resultMsg.sendToTarget()
                return true
            }

            // 设置页「添加图片」：界面里的 <input type=file>
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

        Thread {
            val url = try {
                Core.start(this)
            } catch (e: Exception) {
                Log.e(Core.TAG, "启动失败", e)
                runOnUiThread { failed(e) }
                return@Thread
            }
            runOnUiThread {
                if (isDestroyed) return@runOnUiThread
                origin = originOf(Uri.parse(url))
                web.loadUrl(url)
            }
        }.start()
    }

    private fun failed(e: Exception) {
        if (isFinishing) return
        AlertDialog.Builder(this)
            .setTitle("打不开")
            .setMessage(e.message ?: e.toString())
            .setPositiveButton("退出") { _, _ -> finish() }
            .setCancelable(false)
            .show()
    }

    private fun openOutside(u: Uri) {
        try {
            startActivity(Intent(Intent.ACTION_VIEW, u))
        } catch (e: ActivityNotFoundException) {
        }
    }

    private fun askNotifications() {
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1)
        }
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

    // 界面没有返回的上一页；返回键退到后台，本机运行时定时任务继续跑
    @Deprecated("Activity 的回调")
    override fun onBackPressed() {
        moveTaskToBack(true)
    }

    override fun onDestroy() {
        if (::web.isInitialized) web.destroy()
        super.onDestroy()
    }

    companion object {
        private const val PICK_FILES = 2

        fun originOf(u: Uri) = "${u.scheme}://${u.encodedAuthority}"
    }
}
