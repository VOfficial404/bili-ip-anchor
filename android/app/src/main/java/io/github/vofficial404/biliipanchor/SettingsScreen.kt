package io.github.vofficial404.biliipanchor

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

private data class Level(val value: String, val label: String, val hint: String)

private val LEVELS = listOf(
    Level("debug", "调试（debug）", "记录所有日志，包括每一个发给 B 站的请求，排查问题时再打开"),
    Level("info", "信息（info）", "每轮运行、登录、设置变更等常规记录，推荐"),
    Level("warn", "警告（warn）", "只记录警告和错误"),
    Level("error", "错误（error）", "只记录错误"),
)

private const val MAX_IMAGES = 9

/** 设置：所有账号共用，保存后立即生效。动态图片上传、删除时立即生效。 */
@Composable
fun SettingsScreen(vm: AppViewModel, s: Status, pickImages: () -> Unit) {
    val f = vm.form
    Column(Modifier.fillMaxSize()) {
        Column(
            Modifier.weight(1f).verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Muted("所有账号共用，保存后立即生效。")
            ScheduleSection(vm, f)
            VideosSection(vm, f)
            DynamicSection(vm, s, f, pickImages)
            ProxySection(vm, f)
            ConnectionSection(vm, s)
        }
        if (vm.edited) SaveBar(vm)
    }
}

@Composable
private fun SaveBar(vm: AppViewModel) {
    Surface(tonalElevation = 3.dp, shadowElevation = 3.dp) {
        Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
            if (vm.formError.isNotEmpty()) {
                Text(vm.formError, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("有未保存的修改", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
                TextButton(onClick = { vm.resetForm() }, enabled = !vm.saving) { Text("撤销") }
                Spacer(Modifier.width(8.dp))
                Button(onClick = { vm.saveSettings() }, enabled = !vm.saving) {
                    if (vm.saving) ButtonSpinner()
                    Text("保存")
                }
            }
        }
    }
}

@Composable
private fun ScheduleSection(vm: AppViewModel, f: Settings) {
    Section("运行") {
        OutlinedTextField(
            value = f.interval,
            onValueChange = { vm.updateForm(f.copy(interval = it)) },
            label = { Text("每轮间隔") },
            supportingText = { Text("写成 90m、6h、12h 这样，最少 10m。") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Ascii),
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = f.jitter,
            onValueChange = { vm.updateForm(f.copy(jitter = it)) },
            label = { Text("随机抖动") },
            supportingText = { Text("每轮额外随机等 0 到这么久，让时间不那么规律。") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Ascii),
            modifier = Modifier.fillMaxWidth(),
        )
        Text("日志级别", style = MaterialTheme.typography.labelLarge)
        Column {
            LEVELS.forEach { l ->
                val selected = f.logLevel == l.value
                Row(
                    Modifier
                        .fillMaxWidth()
                        .clip(MaterialTheme.shapes.small)
                        .selectable(selected = selected, role = Role.RadioButton) { vm.updateForm(f.copy(logLevel = l.value)) }
                        .padding(vertical = 2.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    RadioButton(selected = selected, onClick = null)
                    Spacer(Modifier.width(8.dp))
                    Text(l.label, style = MaterialTheme.typography.bodyMedium)
                }
            }
        }
        val hint = LEVELS.firstOrNull { it.value == f.logLevel }?.hint.orEmpty()
        Muted("$hint。调试级别会把每一个 B 站请求都记进日志，日志很快会被刷满。")
    }
}

@Composable
private fun VideosSection(vm: AppViewModel, f: Settings) {
    var text by rememberSaveable { mutableStateOf("") }
    val add = { if (text.isNotBlank() && !vm.addingVideos) vm.addVideos(text) { text = "" } }
    Section("心跳视频", "每轮从下面随机选一个视频上报观看心跳。任意公开视频都可以，最多 20 个。") {
        if (f.videos.isEmpty()) Muted("还没有视频，至少添加一个。")
        f.videos.forEach { bvid ->
            VideoRow(vm, bvid, canRemove = f.videos.size > 1) {
                vm.updateForm(f.copy(videos = f.videos - bvid))
            }
        }
        Row(verticalAlignment = Alignment.Top) {
            OutlinedTextField(
                value = text,
                onValueChange = {
                    text = it
                    vm.videoError = ""
                },
                label = { Text("添加视频") },
                placeholder = { Text("BV 号或视频链接") },
                singleLine = true,
                enabled = !vm.addingVideos,
                isError = vm.videoError.isNotEmpty(),
                supportingText = if (vm.videoError.isNotEmpty()) {
                    { Text(vm.videoError) }
                } else {
                    null
                },
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Done),
                keyboardActions = KeyboardActions(onDone = { add() }),
                modifier = Modifier.weight(1f),
            )
            OutlinedButton(
                onClick = { add() },
                enabled = text.isNotBlank() && !vm.addingVideos,
                modifier = Modifier.padding(start = 8.dp, top = 8.dp),
            ) {
                if (vm.addingVideos) ButtonSpinner()
                Text("添加")
            }
        }
    }
}

@Composable
private fun VideoRow(vm: AppViewModel, bvid: String, canRemove: Boolean, onRemove: () -> Unit) {
    val context = LocalContext.current
    LaunchedEffect(bvid) { vm.loadVideo(bvid) }
    val r = vm.videos[bvid]
    val info = r?.info
    Row(
        Modifier.fillMaxWidth().clip(MaterialTheme.shapes.small).clickable { openUrl(context, videoUrl(bvid)) },
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val url = info?.let { hdslbUrl(it.cover, "@192w_120h_1c.webp") }.orEmpty()
        val cover = rememberBitmap(url, 192) { Api.image(url) }
        Box(
            Modifier.width(96.dp).height(60.dp).clip(MaterialTheme.shapes.small)
                .background(MaterialTheme.colorScheme.surfaceVariant),
        ) {
            if (cover != null) {
                Image(cover, contentDescription = null, modifier = Modifier.fillMaxSize(), contentScale = ContentScale.Crop)
            }
        }
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(
                info?.title?.ifEmpty { null } ?: bvid,
                style = MaterialTheme.typography.bodyMedium,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            when {
                info != null -> Muted(
                    listOf(info.owner, fmtDuration(info.duration), "${fmtCount(info.views)} 播放")
                        .filter { it.isNotEmpty() }.joinToString(" · "),
                )
                r != null && r.error.isNotEmpty() -> Muted(r.error, color = MaterialTheme.colorScheme.error)
                else -> Muted("正在查询…")
            }
            if (info != null) {
                Text(bvid, style = MaterialTheme.typography.labelSmall, fontFamily = FontFamily.Monospace,
                    color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        if (canRemove) {
            IconButton(onClick = onRemove) { Icon(Icons.Filled.Close, contentDescription = "移除这个视频") }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun DynamicSection(vm: AppViewModel, s: Status, f: Settings, pickImages: () -> Unit) {
    Section("动态") {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text("发动态", style = MaterialTheme.typography.bodyLarge)
                Muted("每轮发一条动态，几秒后删掉。只发心跳不能改变属地时再打开；动态会短暂出现在粉丝的动态流里。")
            }
            Spacer(Modifier.width(12.dp))
            Switch(checked = f.doDynamic, onCheckedChange = { vm.updateForm(f.copy(doDynamic = it)) })
        }
        OutlinedTextField(
            value = f.dynamicText,
            onValueChange = { if (it.length <= 1000) vm.updateForm(f.copy(dynamicText = it)) },
            label = { Text("文字") },
            placeholder = { Text("可以不写") },
            minLines = 3,
            modifier = Modifier.fillMaxWidth(),
        )
        Text("图片", style = MaterialTheme.typography.labelLarge)
        Muted("最多 $MAX_IMAGES 张，每张不超过 10 MB。上传和删除立即生效，不需要保存。")
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            s.images.forEach { img -> ImageTile(vm, img.name) }
        }
        OutlinedButton(onClick = pickImages, enabled = !vm.uploading && s.images.size < MAX_IMAGES) {
            if (vm.uploading) ButtonSpinner()
            Text(if (vm.uploading) "正在上传" else "添加图片")
        }
        Surface(color = MaterialTheme.colorScheme.surfaceVariant, shape = MaterialTheme.shapes.small) {
            Text(
                dynamicPreview(f, s.images.size),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
            )
        }
    }
}

private fun dynamicPreview(f: Settings, images: Int): String {
    if (!f.doDynamic) return "不发动态，只上报观看心跳。"
    val text = f.dynamicText.isNotBlank()
    return when {
        text && images > 0 -> "每轮发一条带文字和 $images 张图片的动态。"
        text -> "每轮发一条纯文字动态。"
        images > 0 -> "每轮发一条带 $images 张图片的动态。"
        else -> "没有文字和图片，每轮发内置的一张图片。"
    }
}

@Composable
private fun ImageTile(vm: AppViewModel, name: String) {
    val img = rememberBitmap(vm.imageKey(name), 240) { vm.imageBytes(name) }
    val deleting = vm.deletingImage == name
    Box(Modifier.size(96.dp).clip(MaterialTheme.shapes.small).background(MaterialTheme.colorScheme.surfaceVariant)) {
        if (img != null) {
            Image(img, contentDescription = name, modifier = Modifier.fillMaxSize(), contentScale = ContentScale.Crop)
        }
        Box(
            Modifier.align(Alignment.TopEnd).padding(4.dp).size(28.dp).clip(CircleShape)
                .background(MaterialTheme.colorScheme.scrim.copy(alpha = 0.55f))
                .clickable(enabled = vm.deletingImage.isEmpty()) { vm.deleteImage(name) },
            contentAlignment = Alignment.Center,
        ) {
            if (deleting) {
                CircularProgressIndicator(Modifier.size(14.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.inverseOnSurface)
            } else {
                Icon(
                    Icons.Filled.Close,
                    contentDescription = "删除这张图片",
                    modifier = Modifier.size(16.dp),
                    tint = MaterialTheme.colorScheme.inverseOnSurface,
                )
            }
        }
    }
}

@Composable
private fun ProxySection(vm: AppViewModel, f: Settings) {
    val local = vm.appState.mode == "local"
    Section(
        "代理",
        "所有以账号身份发出的 B 站请求都经过它，B 站看到的就是代理的出口 IP。支持 http://、socks5://，例如 http://127.0.0.1:7890。",
    ) {
        Row(verticalAlignment = Alignment.Top) {
            OutlinedTextField(
                value = f.proxy,
                onValueChange = {
                    vm.updateForm(f.copy(proxy = it))
                    vm.proxyProbe = null
                },
                label = { Text("代理") },
                placeholder = { Text(if (local) "http://127.0.0.1:7890" else "留空表示直连") },
                singleLine = true,
                textStyle = LocalTextStyle.current.copy(fontFamily = FontFamily.Monospace),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                modifier = Modifier.weight(1f),
            )
            OutlinedButton(
                onClick = { vm.testProxy() },
                enabled = !vm.testingProxy,
                modifier = Modifier.padding(start = 8.dp, top = 8.dp),
            ) { Text("测试") }
        }
        vm.proxyProbe?.let { ProbeText(it) }
        if (local && f.proxy.isBlank()) {
            Muted("留空时直接从手机发出，B 站看到的是手机自己的 IP。", color = LocalExtraColors.current.warning)
        }
    }
}

@Composable
private fun ConnectionSection(vm: AppViewModel, s: Status) {
    val local = vm.appState.mode == "local"
    var confirmBack by remember { mutableStateOf(false) }
    var confirmStop by remember { mutableStateOf(false) }
    Section("连接", if (local) "定时任务在这台手机上运行。" else "定时任务在服务器上运行：${vm.appState.server}") {
        if (local) {
            if (vm.stopped) {
                Button(onClick = { vm.restartLocal() }, enabled = !vm.restarting, modifier = Modifier.fillMaxWidth()) {
                    if (vm.restarting) ButtonSpinner()
                    Text("重新开始本机运行")
                }
            } else {
                OutlinedButton(onClick = { confirmStop = true }, modifier = Modifier.fillMaxWidth()) { Text("停止本机运行") }
            }
        }
        OutlinedButton(onClick = { confirmBack = true }, modifier = Modifier.fillMaxWidth()) { Text("回到首页") }
        Muted("回到首页可以换一种用法。" + if (local) "会先停止本机运行，账号和设置都保留。" else "会退出这台服务器的面板登录。")
        HorizontalDivider()
        Muted("Bili IP Anchor ${s.version.ifEmpty { vm.appState.version }}")
    }

    if (confirmStop) {
        ConfirmDialog(
            title = "停止本机运行？",
            text = "停止后定时任务不再运行，直到重新开始。账号和设置都保留。",
            confirm = "停止",
            destructive = true,
            onConfirm = { vm.stopLocal() },
            onDismiss = { confirmStop = false },
        )
    }
    if (confirmBack) {
        ConfirmDialog(
            title = "回到首页？",
            text = if (local) "会停止本机运行，账号和设置都保留，下次选「在手机上运行」时接着用。" else "会退出这台服务器的面板登录，服务器上的定时任务不受影响。",
            confirm = "回到首页",
            onConfirm = { vm.disconnect() },
            onDismiss = { confirmBack = false },
        )
    }
}
