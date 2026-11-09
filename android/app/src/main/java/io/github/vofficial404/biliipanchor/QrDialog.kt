package io.github.vofficial404.biliipanchor

import android.graphics.BitmapFactory
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.FilterQuality
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext

private enum class QrPhase { Loading, Waiting, Scanned, Expired, Failed }

/**
 * 扫码对话框。target.mid 为 null 时是网页扫码登录（添加账号或重新登录），
 * 否则是 TV 扫码授权，用来查询这个账号的主页属地。
 */
@Composable
fun QrDialog(vm: AppViewModel, target: QrTarget, onClose: () -> Unit) {
    val path = if (target.mid == null) "/api/qr" else "/api/accounts/${target.mid}/appauth"
    var round by remember { mutableIntStateOf(0) }
    var phase by remember { mutableStateOf(QrPhase.Loading) }
    var image by remember { mutableStateOf<ImageBitmap?>(null) }
    var error by remember { mutableStateOf("") }

    LaunchedEffect(round) {
        phase = QrPhase.Loading
        error = ""
        val r = vm.call("POST", path)
        if (!r.ok) {
            error = r.message
            phase = QrPhase.Failed
            return@LaunchedEffect
        }
        val bmp = withContext(Dispatchers.Default) { BitmapFactory.decodeByteArray(r.body, 0, r.body.size) }
        if (bmp == null) {
            error = "二维码不是图片"
            phase = QrPhase.Failed
            return@LaunchedEffect
        }
        image = bmp.asImageBitmap()
        phase = QrPhase.Waiting
        while (true) {
            delay(2000)
            val p = vm.call("GET", path)
            if (!p.ok) {
                // 服务器要求重新输入密码时会弹出密码框；其他错误先显示，接着查
                if (p.status == 401) return@LaunchedEffect
                error = p.message
                continue
            }
            error = ""
            val o = p.obj() ?: continue
            when (o.optString("state")) {
                "scanned" -> phase = QrPhase.Scanned
                "expired", "none" -> {
                    phase = QrPhase.Expired
                    return@LaunchedEffect
                }
                "success" -> {
                    onClose()
                    vm.qrDone(target, o.optString("mid"))
                    return@LaunchedEffect
                }
                else -> phase = QrPhase.Waiting
            }
        }
    }

    val title = if (target.mid == null) "扫码登录 B 站账号" else "授权查询「${target.label}」的主页属地"
    val hint = if (target.mid == null) {
        "用 B 站 App 的「扫一扫」扫码并确认登录。"
    } else {
        "用「${target.label}」这个账号的 B 站 App 扫码并确认。只用来查询主页显示的 IP 属地，可以随时取消。"
    }
    val stateText = when (phase) {
        QrPhase.Loading -> "正在获取二维码…"
        QrPhase.Waiting -> "等待扫码"
        QrPhase.Scanned -> "已扫码，请在 App 里确认"
        QrPhase.Expired -> "二维码已过期"
        QrPhase.Failed -> "获取二维码失败"
    }
    val again = phase == QrPhase.Expired || phase == QrPhase.Failed

    AlertDialog(
        onDismissRequest = onClose,
        title = { Text(title) },
        text = {
            Column(
                Modifier.fillMaxWidth(),
                horizontalAlignment = Alignment.CenterHorizontally,
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                // 二维码在深色模式下也要白底
                Box(
                    Modifier.size(232.dp).clip(RoundedCornerShape(12.dp)).background(Color.White).padding(8.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    val img = image
                    if (img != null && phase != QrPhase.Loading && phase != QrPhase.Failed) {
                        Image(
                            img,
                            contentDescription = "登录二维码",
                            filterQuality = FilterQuality.None,
                            modifier = Modifier.fillMaxSize().alpha(if (again || phase == QrPhase.Scanned) 0.25f else 1f),
                        )
                    } else if (phase == QrPhase.Loading) {
                        CircularProgressIndicator()
                    }
                }
                Text(
                    stateText,
                    style = MaterialTheme.typography.titleSmall,
                    color = if (again) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface,
                )
                if (error.isNotEmpty()) {
                    Text(
                        error,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                        textAlign = TextAlign.Center,
                    )
                }
                Muted(hint)
                Muted("二维码就在这台手机上时，可以截图后在「扫一扫」里从相册选这张图，或者用另一台手机扫。")
            }
        },
        confirmButton = {
            if (again) TextButton(onClick = { round++ }) { Text("刷新二维码") }
        },
        dismissButton = { TextButton(onClick = onClose) { Text(if (again) "关闭" else "取消") } },
    )
}
