package io.github.vofficial404.biliipanchor

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ExitToApp
import androidx.compose.material.icons.filled.DateRange
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.LocationOn
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Place
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlin.math.ceil

/** 账号：每个账号一张卡片 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AccountsScreen(
    vm: AppViewModel,
    s: Status,
    openQr: (QrTarget) -> Unit,
    exportFav: (Account) -> Unit,
    importFav: (Account) -> Unit,
) {
    PullToRefreshBox(isRefreshing = vm.refreshing, onRefresh = { vm.pullRefresh() }, modifier = Modifier.fillMaxSize()) {
        LazyColumn(
            Modifier.fillMaxSize(),
            // 底部留出「添加账号」按钮的位置
            contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 16.dp, bottom = 96.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            if (s.accounts.isEmpty()) {
                item { EmptyAccounts(openQr) }
            }
            items(s.accounts, key = { it.mid }) { a ->
                AccountCard(vm, s, a, openQr, exportFav, importFav)
            }
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun AccountCard(
    vm: AppViewModel,
    s: Status,
    a: Account,
    openQr: (QrTarget) -> Unit,
    exportFav: (Account) -> Unit,
    importFav: (Account) -> Unit,
) {
    val busy = a.mid in vm.pending || a.running || a.busy.isNotEmpty()
    var confirmLogout by remember { mutableStateOf(false) }
    var confirmUnauth by remember { mutableStateOf(false) }

    OutlinedCard(Modifier.fillMaxWidth()) {
        // 头像、名字、状态和资料
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
            Avatar(a, 48.dp)
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        a.label,
                        style = MaterialTheme.typography.titleMedium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                    Spacer(Modifier.width(8.dp))
                    val (text, tone) = accountState(a)
                    Pill(text, tone)
                }
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Pill("UID ${a.mid}", Tone.Muted)
                    a.stats?.let { st ->
                        val levelTone = when {
                            st.level >= 6 -> Tone.Danger
                            st.level == 5 -> Tone.Warning
                            else -> Tone.Muted
                        }
                        Pill("Lv${st.level}", levelTone)
                        if (st.vip.isNotEmpty()) Pill(st.vip, MaterialTheme.colorScheme.primary)
                        Pill("硬币 ${fmtNumber(st.coins)}", Tone.Muted)
                        if (st.bcoins > 0) Pill("B币 ${fmtNumber(st.bcoins)}", Tone.Muted)
                    }
                }
            }
        }

        // 操作
        Row(
            Modifier.fillMaxWidth().padding(start = 16.dp, end = 8.dp, bottom = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            if (a.loggedIn) {
                Button(onClick = { vm.accountAction(a, "POST", "run", null) }, enabled = !busy, modifier = Modifier.weight(1f)) {
                    Icon(Icons.Filled.PlayArrow, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(8.dp))
                    Text(if (a.running) "运行中" else "立即运行")
                }
            } else {
                Button(onClick = { openQr(QrTarget(null, "")) }, modifier = Modifier.weight(1f)) { Text("重新扫码") }
            }
            IconButton(onClick = { vm.accountAction(a, "POST", "check", "资料已刷新") }, enabled = !busy && a.loggedIn) {
                Icon(Icons.Filled.Refresh, contentDescription = "刷新资料")
            }
            IconButton(onClick = { confirmLogout = true }, enabled = a.mid !in vm.pending) {
                Icon(
                    Icons.AutoMirrored.Filled.ExitToApp,
                    contentDescription = "退出",
                    tint = MaterialTheme.colorScheme.error,
                )
            }
        }

        HorizontalDivider()
        HomeRow(a, s, busy, openQr) { confirmUnauth = true }
        TargetRow(s)
        RunRow(a)
        CookieRow(vm, a, busy)

        HorizontalDivider()
        FavoritesRow(vm, a, busy, exportFav, importFav)

        val error = if (!a.loggedIn) {
            "这个账号的会话已失效，点「重新扫码」用同一个账号登录即可恢复。"
        } else {
            a.last?.takeIf { !it.ok }?.error.orEmpty()
        }
        if (error.isNotEmpty()) {
            HorizontalDivider()
            Text(
                error,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.padding(16.dp),
            )
        }
    }

    if (confirmLogout) {
        ConfirmDialog(
            title = "退出「${a.label}」？",
            text = "这份会话会在 B 站注销并删除，之后不再为这个账号运行。",
            confirm = "退出",
            destructive = true,
            onConfirm = { vm.accountAction(a, "DELETE", "", "已退出 ${a.label}") },
            onDismiss = { confirmLogout = false },
        )
    }
    if (confirmUnauth) {
        ConfirmDialog(
            title = "取消「${a.label}」的主页属地授权？",
            text = "会在 B 站撤销这份 App 授权，之后不再显示主页属地。不影响定时任务。",
            confirm = "取消授权",
            onConfirm = { vm.accountAction(a, "DELETE", "appauth", "已取消授权") },
            onDismiss = { confirmUnauth = false },
        )
    }
}

/** 卡片里的一行信息 */
@Composable
private fun InfoRow(
    icon: ImageVector,
    title: String,
    value: String,
    sub: String = "",
    pending: Boolean = false,
    valueColor: Color = Color.Unspecified,
    badge: (@Composable () -> Unit)? = null,
    action: (@Composable () -> Unit)? = null,
) {
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
        Icon(icon, contentDescription = null, modifier = Modifier.size(20.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.width(16.dp))
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Muted(title)
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    value,
                    style = MaterialTheme.typography.bodyLarge,
                    fontWeight = if (pending) FontWeight.Normal else FontWeight.SemiBold,
                    color = when {
                        valueColor != Color.Unspecified -> valueColor
                        pending -> MaterialTheme.colorScheme.onSurfaceVariant
                        else -> Color.Unspecified
                    },
                )
                if (badge != null) {
                    Spacer(Modifier.width(8.dp))
                    badge()
                }
            }
            if (sub.isNotEmpty()) Muted(sub)
        }
        if (action != null) {
            Spacer(Modifier.width(8.dp))
            action()
        }
    }
}

@Composable
private fun HomeRow(a: Account, s: Status, busy: Boolean, openQr: (QrTarget) -> Unit, unauth: () -> Unit) {
    val target = s.zone?.let { zoneRegion(it) }.orEmpty()
    val h = a.home
    var value: String
    var sub = ""
    var pending = false
    var badge: (@Composable () -> Unit)? = null
    when {
        !a.appAuth -> {
            value = "未授权查询"
            pending = true
            sub = "B 站只在 App 接口里提供主页属地，需要额外授权一次才能显示。"
        }
        h == null || (h.text.isEmpty() && h.error.isEmpty()) -> {
            value = "查询中"
            pending = true
        }
        h.text.isEmpty() -> {
            value = "查不到"
            pending = true
            sub = h.error
        }
        else -> {
            value = h.text
            if (h.matchedSince != null) {
                badge = { Pill("已是目标属地", Tone.Success) }
                sub = "${fmtTime(h.matchedSince)} 起一致"
            } else if (target.isNotEmpty()) {
                badge = { Pill("还不是$target", Tone.Warning) }
                val next = a.next?.let { "下一轮（${fmtTime(it)}）" } ?: "下一轮"
                sub = "预计${next}之后几小时内变为$target，B 站没有公开具体规则"
            }
            if (h.error.isNotEmpty()) {
                sub = "上次查询失败：${h.error}"
            } else if (h.checkedAt != null) {
                sub += (if (sub.isNotEmpty()) "。" else "") + "查询于 ${fmtTime(h.checkedAt)}"
            }
        }
    }
    InfoRow(
        Icons.Filled.Place, "主页属地", value, sub, pending, badge = badge,
        action = {
            if (a.appAuth) {
                TextButton(onClick = unauth, enabled = !busy) { Text("取消授权") }
            } else {
                OutlinedButton(onClick = { openQr(QrTarget(a.mid, a.label)) }, enabled = !busy && a.loggedIn) { Text("授权查询") }
            }
        },
    )
}

@Composable
private fun TargetRow(s: Status) {
    val z = s.zone
    InfoRow(
        Icons.Filled.LocationOn, "目标属地",
        value = z?.let { zoneRegion(it).ifEmpty { "未知" } } ?: "查询中",
        sub = z?.let { listOf(it.addr, it.isp).filter { p -> p.isNotEmpty() }.joinToString(" · ") }.orEmpty(),
        pending = z == null,
    )
}

@Composable
private fun RunRow(a: Account) {
    val lines = mutableListOf<String>()
    if (a.next != null && !a.running) lines.add(fmtRelative(a.next))
    a.last?.let { l -> l.at?.let { lines.add("上次 ${fmtTime(it)} ${if (l.ok) "成功" else "失败"}") } }
    a.lastOk?.let { lines.add("上次从目标 IP 发出请求：${fmtTime(it)}") }
    InfoRow(
        Icons.Filled.DateRange, "运行",
        value = when {
            a.running -> "正在运行"
            a.next != null -> "下次 ${fmtTime(a.next)}"
            else -> "—"
        },
        sub = lines.joinToString(" · ").ifEmpty { "还没有运行过" },
    )
}

@Composable
private fun CookieRow(vm: AppViewModel, a: Account, busy: Boolean) {
    val exp = a.cookieExpires
    val value: String
    val sub: String
    var color = Color.Unspecified
    var pending = false
    if (exp != null) {
        val days = ceil((exp - System.currentTimeMillis()) / 86_400_000.0).toLong()
        value = if (days > 0) "还有 $days 天" else "已过期"
        if (days <= 0) color = MaterialTheme.colorScheme.error
        else if (days < 14) color = LocalExtraColors.current.warning
        sub = "到 ${fmtDate(exp)}" + if (a.canRefresh) "，B 站要求时会自动刷新" else ""
    } else {
        value = "有效期未知"
        pending = true
        sub = if (a.canRefresh) "下次运行时会查询" else "旧版本扫码登录的账号，重新扫码后可以看到有效期并刷新"
    }
    val refreshing = a.busy == "刷新 Cookie"
    InfoRow(
        Icons.Filled.Lock, "Cookie", value, sub, pending, valueColor = color,
        action = {
            OutlinedButton(
                onClick = { vm.accountAction(a, "POST", "cookie/refresh", "Cookie 已刷新") },
                enabled = !busy && a.loggedIn && a.canRefresh,
            ) {
                if (refreshing) ButtonSpinner()
                Text(if (refreshing) "正在刷新" else "刷新")
            }
        },
    )
}

/** 收藏夹：导出成文件、从文件导入、导入进度 */
@Composable
private fun FavoritesRow(
    vm: AppViewModel,
    a: Account,
    busy: Boolean,
    exportFav: (Account) -> Unit,
    importFav: (Account) -> Unit,
) {
    val idle = !busy && a.loggedIn && vm.exporting.isEmpty()
    Column(Modifier.fillMaxWidth().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(
                Icons.Filled.Favorite,
                contentDescription = null,
                modifier = Modifier.size(20.dp),
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.width(16.dp))
            Column(Modifier.weight(1f)) {
                Text("收藏夹", style = MaterialTheme.typography.titleSmall)
                Muted("导出成文件，可以导入到这个或别的账号")
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            OutlinedButton(onClick = { exportFav(a) }, enabled = idle, modifier = Modifier.weight(1f)) {
                if (vm.exporting == a.mid) {
                    ButtonSpinner()
                    Text("正在导出")
                } else {
                    Text("导出")
                }
            }
            OutlinedButton(onClick = { importFav(a) }, enabled = idle, modifier = Modifier.weight(1f)) { Text("导入") }
        }
        a.fav?.let { ImportProgress(it) }
    }
}

@Composable
private fun ImportProgress(job: FavImport) {
    val context = LocalContext.current
    var open by remember { mutableStateOf(false) }
    val failed = job.error.isNotEmpty()
    val head = when {
        job.running -> if (job.folder.isNotEmpty()) "正在导入「${job.folder}」" else "正在导入"
        failed -> "导入中途停下"
        else -> "导入完成"
    }
    val counts = buildList {
        add("新增 ${job.added}")
        add("已有 ${job.skipped}")
        if (job.failed > 0) add("失败 ${job.failed}")
        if (job.created.isNotEmpty()) add("新建了" + job.created.joinToString("") { "「$it」" })
    }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Row {
            Text(
                head,
                style = MaterialTheme.typography.labelLarge,
                color = if (failed) MaterialTheme.colorScheme.error else Color.Unspecified,
                modifier = Modifier.weight(1f),
            )
            Muted("${job.done}/${job.total}" + (job.finished?.let { " · ${fmtTime(it)}" } ?: ""))
        }
        LinearProgressIndicator(
            progress = { if (job.total > 0) job.done.toFloat() / job.total else 1f },
            modifier = Modifier.fillMaxWidth(),
            color = if (failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary,
        )
        Muted(counts.joinToString(" · "))
        if (failed) Muted(job.error, color = MaterialTheme.colorScheme.error)
        if (job.failures.isNotEmpty()) {
            TextButton(onClick = { open = !open }, contentPadding = PaddingValues(0.dp)) {
                Text(if (open) "收起没有收藏上的视频" else "没有收藏上的视频（${job.failures.size}）")
            }
            if (open) {
                job.failures.forEach { f ->
                    Column(Modifier.fillMaxWidth().clickable { openUrl(context, videoUrl(f.bvid)) }.padding(vertical = 4.dp)) {
                        Text(
                            f.bvid,
                            style = MaterialTheme.typography.bodySmall,
                            fontFamily = FontFamily.Monospace,
                            color = MaterialTheme.colorScheme.primary,
                        )
                        Muted(listOf(f.title, f.reason).filter { it.isNotEmpty() }.joinToString(" — "))
                    }
                }
            }
        }
    }
}
