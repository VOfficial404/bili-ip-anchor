package io.github.vofficial404.biliipanchor

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.horizontalScroll
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import kotlinx.coroutines.delay

private val LEVEL_RANK = mapOf("debug" to 0, "info" to 1, "warn" to 2, "error" to 3)

private fun rank(level: String) = LEVEL_RANK[level.lowercase()] ?: 1

/** 日志：最新的在最下面，停在底部时跟着新日志滚动 */
@Composable
fun LogsScreen(vm: AppViewModel) {
    var min by rememberSaveable { mutableStateOf("debug") }
    val list = rememberLazyListState()

    val lifecycle = LocalLifecycleOwner.current.lifecycle
    LaunchedEffect(lifecycle) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
            while (true) {
                vm.pollLogs()
                delay(3000)
            }
        }
    }

    val shown = remember(vm.logs, min) { vm.logs.filter { rank(it.level) >= rank(min) } }
    val atBottom by remember {
        derivedStateOf {
            val info = list.layoutInfo
            val last = info.visibleItemsInfo.lastOrNull()
            last == null || last.index >= info.totalItemsCount - 2
        }
    }
    LaunchedEffect(shown.size) {
        if (shown.isNotEmpty() && atBottom) list.scrollToItem(shown.size - 1)
    }

    Column(Modifier.fillMaxSize()) {
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            listOf("debug" to "全部", "info" to "信息及以上", "warn" to "警告及以上", "error" to "只看错误").forEach { (v, label) ->
                FilterChip(selected = min == v, onClick = { min = v }, label = { Text(label) })
            }
        }
        if (vm.logError.isNotEmpty()) {
            Text(
                vm.logError,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.padding(horizontal = 16.dp),
            )
        }
        HorizontalDivider()
        if (shown.isEmpty()) {
            Box(Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
                Muted(if (vm.logs.isEmpty()) "还没有日志" else "这个级别没有日志")
            }
        } else {
            LazyColumn(
                state = list,
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(vertical = 8.dp),
            ) {
                items(shown, key = { it.seq }) { e -> LogRow(e) }
            }
        }
    }
}

@Composable
private fun LogRow(e: LogEntry) {
    val level = e.level.lowercase()
    val color = when (level) {
        "error" -> MaterialTheme.colorScheme.error
        "warn" -> LocalExtraColors.current.warning
        "debug" -> MaterialTheme.colorScheme.onSurfaceVariant
        else -> LocalExtraColors.current.info
    }
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                level.uppercase(),
                style = MaterialTheme.typography.labelSmall,
                fontFamily = FontFamily.Monospace,
                fontWeight = FontWeight.SemiBold,
                color = color,
            )
            Spacer(Modifier.width(8.dp))
            e.time?.let {
                Text(
                    fmtLogTime(it),
                    style = MaterialTheme.typography.labelSmall,
                    fontFamily = FontFamily.Monospace,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            val who = e.account.ifEmpty { e.mid }
            if (who.isNotEmpty()) {
                Spacer(Modifier.width(8.dp))
                Text(who, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.primary, maxLines = 1)
            }
        }
        Text(
            e.msg,
            style = MaterialTheme.typography.bodySmall,
            color = if (level == "debug") MaterialTheme.colorScheme.onSurfaceVariant else Color.Unspecified,
        )
    }
}
