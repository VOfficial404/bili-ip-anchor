package io.github.vofficial404.biliipanchor

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.util.Log
import mobile.Mobile

/**
 * 本机运行时的前台服务：显示一条常驻通知，让系统不回收进程，定时任务在后台继续跑。
 * 进程被回收后系统会重启它（START_STICKY），这时用记住的代理重新开始运行。
 */
class AnchorService : Service() {
    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val proxy = Prefs(this).proxy
        try {
            foreground(proxy)
        } catch (e: Exception) {
            Log.w(SetupActivity.TAG, "前台服务启动失败", e)
            stopSelf()
            return START_NOT_STICKY
        }
        if (!Mobile.running()) {
            Thread {
                try {
                    Local.start(this, proxy)
                } catch (e: Exception) {
                    Log.w(SetupActivity.TAG, "本机运行失败", e)
                    stopSelf()
                }
            }.start()
        }
        return START_STICKY
    }

    override fun onDestroy() {
        Mobile.stop()
        super.onDestroy()
    }

    private fun foreground(proxy: String) {
        val builder = if (Build.VERSION.SDK_INT >= 26) {
            val nm = getSystemService(NotificationManager::class.java)
            nm.createNotificationChannel(NotificationChannel(CHANNEL, "本机运行", NotificationManager.IMPORTANCE_LOW))
            Notification.Builder(this, CHANNEL)
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(this)
        }
        val open = PendingIntent.getActivity(
            this, 0,
            Intent(this, SetupActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val n = builder
            .setSmallIcon(R.drawable.ic_notify)
            .setContentTitle("正在本机运行")
            .setContentText("按设置定时以账号身份发请求，经过代理 $proxy")
            .setContentIntent(open)
            .setOngoing(true)
            .build()
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(1, n, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(1, n)
        }
    }

    companion object {
        private const val CHANNEL = "local"

        fun start(context: Context) {
            val intent = Intent(context, AnchorService::class.java)
            if (Build.VERSION.SDK_INT >= 26) context.startForegroundService(intent) else context.startService(intent)
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, AnchorService::class.java))
        }
    }
}
