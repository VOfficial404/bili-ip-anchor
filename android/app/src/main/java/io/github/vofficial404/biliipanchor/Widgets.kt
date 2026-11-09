package io.github.vofficial404.biliipanchor

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import android.util.LruCache
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.selection.toggleable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** 用浏览器或 B 站 App 打开链接 */
fun openUrl(context: Context, url: String) {
    try {
        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
    } catch (e: ActivityNotFoundException) {
        // 没有能打开链接的 App
    }
}

fun videoUrl(bvid: String) = "https://www.bilibili.com/video/$bvid"

/* 图片：头像、封面和动态图片，解码后放在内存里 */

object Bitmaps {
    private val cache = object : LruCache<String, Bitmap>(24 * 1024 * 1024) {
        override fun sizeOf(key: String, value: Bitmap) = value.byteCount
    }

    fun get(key: String): Bitmap? = cache.get(key)

    fun remove(key: String) {
        cache.remove(key)
    }

    suspend fun load(key: String, maxSize: Int, fetch: suspend () -> ByteArray?): Bitmap? {
        cache.get(key)?.let { return it }
        val bytes = try {
            fetch()
        } catch (e: CoreException) {
            null
        } ?: return null
        val bmp = withContext(Dispatchers.Default) { decode(bytes, maxSize) } ?: return null
        cache.put(key, bmp)
        return bmp
    }

    /** 缩小到边长不小于 maxSize 的最小尺寸，大图不占太多内存 */
    private fun decode(bytes: ByteArray, maxSize: Int): Bitmap? {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        if (bounds.outWidth <= 0 || bounds.outHeight <= 0) return null
        var sample = 1
        while (bounds.outWidth / (sample * 2) >= maxSize && bounds.outHeight / (sample * 2) >= maxSize) sample *= 2
        val opts = BitmapFactory.Options().apply { inSampleSize = sample }
        return BitmapFactory.decodeByteArray(bytes, 0, bytes.size, opts)
    }
}

/** 按 key 取图片，没有缓存时用 fetch 下载。key 为空时返回 null。 */
@Composable
fun rememberBitmap(key: String, maxSize: Int, fetch: suspend () -> ByteArray?): ImageBitmap? {
    var bmp by remember(key) { mutableStateOf<Bitmap?>(if (key.isEmpty()) null else Bitmaps.get(key)) }
    LaunchedEffect(key) {
        if (bmp == null && key.isNotEmpty()) bmp = Bitmaps.load(key, maxSize, fetch)
    }
    val b = bmp
    return remember(b) { b?.asImageBitmap() }
}

/** B 站头像，没有或者下载不了时显示首字 */
@Composable
fun Avatar(a: Account, size: Dp) {
    val url = hdslbUrl(a.face, "@96w_96h_1c.webp")
    val img = rememberBitmap(url, 96) { Api.image(url) }
    Box(
        Modifier.size(size).clip(CircleShape).background(Color(avatarColor(a.label))),
        contentAlignment = Alignment.Center,
    ) {
        if (img != null) {
            Image(img, contentDescription = null, modifier = Modifier.fillMaxSize(), contentScale = ContentScale.Crop)
        } else {
            Text(
                initials(a.label),
                color = Color.White,
                fontWeight = FontWeight.SemiBold,
                fontSize = (size.value * 0.4f).sp,
            )
        }
    }
}

/* 状态标签 */

enum class Tone { Info, Danger, Muted, Success, Warning }

@Composable
fun toneColor(t: Tone): Color {
    val extra = LocalExtraColors.current
    return when (t) {
        Tone.Info -> extra.info
        Tone.Danger -> MaterialTheme.colorScheme.error
        Tone.Muted -> MaterialTheme.colorScheme.onSurfaceVariant
        Tone.Success -> extra.success
        Tone.Warning -> extra.warning
    }
}

@Composable
fun Pill(text: String, color: Color) {
    Surface(color = color.copy(alpha = 0.14f), contentColor = color, shape = RoundedCornerShape(50)) {
        Text(
            text,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp),
            style = MaterialTheme.typography.labelMedium,
            maxLines = 1,
        )
    }
}

@Composable
fun Pill(text: String, tone: Tone) = Pill(text, toneColor(tone))

fun accountState(a: Account): Pair<String, Tone> = when {
    a.running -> "运行中" to Tone.Info
    a.busy.isNotEmpty() -> "正在${a.busy}" to Tone.Info
    !a.loggedIn -> "登录失效" to Tone.Danger
    a.last == null -> "等待首次运行" to Tone.Muted
    a.last?.ok == true -> "正常" to Tone.Success
    else -> "上次失败" to Tone.Warning
}

/* 常用的小部件 */

@Composable
fun ConfirmDialog(
    title: String,
    text: String,
    confirm: String,
    destructive: Boolean = false,
    onConfirm: () -> Unit,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = { Text(text) },
        confirmButton = {
            TextButton(
                onClick = {
                    onDismiss()
                    onConfirm()
                },
                colors = if (destructive) {
                    ButtonDefaults.textButtonColors(contentColor = MaterialTheme.colorScheme.error)
                } else {
                    ButtonDefaults.textButtonColors()
                },
            ) { Text(confirm) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}

/** 设置页等处的一组内容 */
@Composable
fun Section(title: String, desc: String = "", content: @Composable ColumnScope.() -> Unit) {
    OutlinedCard(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(title, style = MaterialTheme.typography.titleMedium)
                if (desc.isNotEmpty()) Muted(desc)
            }
            content()
        }
    }
}

/** 小号的说明文字 */
@Composable
fun Muted(text: String, modifier: Modifier = Modifier, color: Color = Color.Unspecified) {
    Text(
        text,
        modifier = modifier,
        style = MaterialTheme.typography.bodySmall,
        color = if (color == Color.Unspecified) MaterialTheme.colorScheme.onSurfaceVariant else color,
    )
}

@Composable
fun CheckRow(text: String, checked: Boolean, enabled: Boolean = true, onChange: (Boolean) -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .clip(MaterialTheme.shapes.small)
            .toggleable(value = checked, enabled = enabled, role = Role.Checkbox, onValueChange = onChange)
            .padding(vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Checkbox(checked = checked, onCheckedChange = null, enabled = enabled)
        Spacer(Modifier.width(8.dp))
        Text(text, style = MaterialTheme.typography.bodyMedium)
    }
}

/** 按钮里的转圈 */
@Composable
fun ButtonSpinner() {
    CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = LocalContentColor.current)
    Spacer(Modifier.width(8.dp))
}

@Composable
fun EmptyAccounts(openQr: (QrTarget) -> Unit) {
    OutlinedCard(Modifier.fillMaxWidth()) {
        Column(
            Modifier.fillMaxWidth().padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Text("还没有账号", style = MaterialTheme.typography.titleMedium)
            Text(
                "扫码登录一个 B 站账号，它就会按设置定时运行。可以添加多个，每个账号各自排程、同时进行。",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(4.dp))
            Button(onClick = { openQr(QrTarget(null, "")) }) { Text("扫码添加第一个账号") }
        }
    }
}
