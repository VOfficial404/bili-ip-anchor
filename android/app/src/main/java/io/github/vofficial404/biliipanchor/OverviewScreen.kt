package io.github.vofficial404.biliipanchor

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

/** 概览：目标属地、运行方式和每个账号的状态 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun OverviewScreen(vm: AppViewModel, s: Status, openQr: (QrTarget) -> Unit, openAccounts: () -> Unit) {
    PullToRefreshBox(isRefreshing = vm.refreshing, onRefresh = { vm.pullRefresh() }, modifier = Modifier.fillMaxSize()) {
        Column(
            Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            ZoneCard(s)
            ModeCard(vm, s)
            if (s.accounts.isEmpty()) {
                EmptyAccounts(openQr)
            } else {
                AccountsSummary(s, openAccounts)
                Button(
                    onClick = { vm.runAll() },
                    enabled = !vm.runningAll && !vm.stopped,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    if (vm.runningAll) ButtonSpinner()
                    Text("全部立即运行")
                }
            }
        }
    }
}

/** B 站看到的出口 IP 和属地 */
@Composable
private fun ZoneCard(s: Status) {
    ElevatedCard(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Muted("目标属地 · ${whereLabel(s)}")
            val z = s.zone
            if (z != null) {
                Text(
                    zoneRegion(z).ifEmpty { "未知" },
                    style = MaterialTheme.typography.displaySmall,
                    fontWeight = FontWeight.SemiBold,
                    color = MaterialTheme.colorScheme.primary,
                )
                Text(
                    listOf(zonePlace(z), z.isp).filter { it.isNotEmpty() }.joinToString(" · "),
                    style = MaterialTheme.typography.bodyLarge,
                )
                Muted(z.addr + (s.zoneCheckedAt?.let { " · 查询于 ${fmtTime(it)}" } ?: ""))
            } else {
                Text(
                    if (s.zoneError.isNotEmpty()) "查不到" else "查询中",
                    style = MaterialTheme.typography.headlineSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (s.zoneError.isNotEmpty()) Muted(s.zoneError, color = MaterialTheme.colorScheme.error)
            Spacer(Modifier.padding(top = 4.dp))
            Muted("账号的主页属地会在定时请求之后几小时内变成这里，B 站没有公开具体规则。")
        }
    }
}

@Composable
private fun ModeCard(vm: AppViewModel, s: Status) {
    val local = vm.appState.mode == "local"
    OutlinedCard(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(if (local) "在手机上运行" else "连接服务器", style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.width(8.dp))
                when {
                    local && vm.stopped -> Pill("已停止", Tone.Danger)
                    local -> Pill("运行中", Tone.Success)
                    else -> Pill("已连接", Tone.Success)
                }
            }
            if (local) {
                val proxy = s.settings.proxy
                Muted(if (proxy.isNotEmpty()) "经过代理 $proxy" else "没有设置代理，B 站看到的是手机自己的 IP")
                Muted("退出界面后在通知栏里继续运行。")
            } else {
                Muted(vm.appState.server)
            }
            Muted("版本 ${s.version.ifEmpty { vm.appState.version }}")
        }
    }
}

@Composable
private fun AccountsSummary(s: Status, openAccounts: () -> Unit) {
    val ok = s.accounts.count { it.loggedIn && it.last?.ok == true }
    val bad = s.accounts.count { !it.loggedIn || it.last?.ok == false }
    OutlinedCard(Modifier.fillMaxWidth()) {
        Column {
            Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("账号", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                Muted(
                    buildList {
                        add("${s.accounts.size} 个")
                        if (ok > 0) add("$ok 个正常")
                        if (bad > 0) add("$bad 个需要处理")
                    }.joinToString(" · "),
                )
            }
            s.accounts.forEach { a ->
                HorizontalDivider()
                Row(
                    Modifier.fillMaxWidth().clickable(onClick = openAccounts).padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Avatar(a, 40.dp)
                    Spacer(Modifier.width(12.dp))
                    Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                        Text(a.label, style = MaterialTheme.typography.bodyLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Muted(runLine(a), modifier = Modifier.fillMaxWidth())
                    }
                    Spacer(Modifier.width(8.dp))
                    val (text, tone) = accountState(a)
                    Pill(text, tone)
                }
            }
        }
    }
}

private fun runLine(a: Account): String {
    val parts = mutableListOf<String>()
    a.home?.text?.takeIf { it.isNotEmpty() }?.let { parts.add(it) }
    if (a.running) {
        parts.add("正在运行")
    } else {
        a.next?.let { parts.add("下次 ${fmtTime(it)}") }
    }
    if (parts.isEmpty()) a.last?.at?.let { parts.add("上次 ${fmtTime(it)}") }
    return parts.joinToString(" · ").ifEmpty { "UID ${a.mid}" }
}
