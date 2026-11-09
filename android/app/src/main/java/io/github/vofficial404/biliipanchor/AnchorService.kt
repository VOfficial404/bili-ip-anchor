package io.github.vofficial404.biliipanchor

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.graphics.drawable.Icon
import android.os.Build
import android.os.IBinder
import android.util.Log
import mobile.Mobile

/**
 * 本机运行时的前台服务：显示一条常驻通知，让系统不回收进程，定时任务在后台继续跑。
 * 由 Core 的 Listener 开关。进程被回收后系统会重启它（START_STICKY），这时重新开始 Go 核心并接着运行。
 */
class AnchorService : Service() {
    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            // 通知里的「停止」：Listener 收到停止后会关掉这个服务；核心没在跑时自己关
            Thread {
                Mobile.stopLocal()
                stopSelf()
            }.start()
            return START_NOT_STICKY
        }
        try {
            foreground()
        } catch (e: Exception) {
            Log.w(Core.TAG, "前台服务启动失败", e)
            stopSelf()
            return START_NOT_STICKY
        }
        if (!Mobile.localRunning()) {
            Thread {
                try {
                    Core.start(this)
                } catch (e: Exception) {
                    Log.w(Core.TAG, "本机运行失败", e)
                }
                if (!Mobile.localRunning()) stopSelf()
            }.start()
        }
        return START_STICKY
    }

    private fun foreground() {
        val builder = if (Build.VERSION.SDK_INT >= 26) {
            val nm = getSystemService(NotificationManager::class.java)
            nm.createNotificationChannel(NotificationChannel(CHANNEL, "本机运行", NotificationManager.IMPORTANCE_LOW))
            Notification.Builder(this, CHANNEL)
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(this)
        }
        val flags = PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        val open = PendingIntent.getActivity(
            this, 0,
            Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            flags,
        )
        val stop = PendingIntent.getService(this, 1, Intent(this, AnchorService::class.java).setAction(ACTION_STOP), flags)
        val n = builder
            .setSmallIcon(R.drawable.ic_notify)
            .setContentTitle("正在本机运行")
            .setContentText("按设置定时以账号身份经过代理发请求")
            .setContentIntent(open)
            .addAction(Notification.Action.Builder(Icon.createWithResource(this, R.drawable.ic_notify), "停止", stop).build())
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
        private const val ACTION_STOP = "stop"

        fun start(context: Context) {
            val intent = Intent(context, AnchorService::class.java)
            try {
                if (Build.VERSION.SDK_INT >= 26) context.startForegroundService(intent) else context.startService(intent)
            } catch (e: Exception) {
                // Android 12 起在后台不能开始前台服务；定时任务仍在跑，只是进程可能被回收
                Log.w(Core.TAG, "开不了前台服务", e)
            }
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, AnchorService::class.java))
        }
    }
}
