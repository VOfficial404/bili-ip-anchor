package io.github.vofficial404.biliipanchor

import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import kotlinx.coroutines.delay
import kotlin.math.max
import kotlin.math.roundToInt

enum class Tab(val label: String, val icon: ImageVector) {
    Overview("概览", Icons.Filled.Home),
    Accounts("账号", Icons.Filled.Person),
    Prefs("设置", Icons.Filled.Settings),
    Logs("日志", Icons.AutoMirrored.Filled.List),
}

/** 整个界面：按阶段显示加载、首页或主界面 */
@Composable
fun App(vm: AppViewModel) {
    val snackbar = remember { SnackbarHostState() }
    LaunchedEffect(Unit) {
        vm.messageFlow.collect { snackbar.showSnackbar(it) }
    }

    when (vm.phase) {
        Phase.Loading -> Box(
            Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background),
            contentAlignment = Alignment.Center,
        ) { CircularProgressIndicator() }
        Phase.Failed -> FailedScreen(vm)
        Phase.Setup -> SetupScreen(vm, snackbar)
        Phase.Main -> MainScreen(vm, snackbar)
    }

    vm.notice?.let { msg ->
        AlertDialog(
            onDismissRequest = { vm.notice = null },
            title = { Text("请注意") },
            text = { Text(msg) },
            confirmButton = { TextButton(onClick = { vm.notice = null }) { Text("知道了") } },
        )
    }
}

@Composable
private fun FailedScreen(vm: AppViewModel) {
    Surface(Modifier.fillMaxSize()) {
        Column(
            Modifier.fillMaxSize().padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp, Alignment.CenterVertically),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text("启动失败", style = MaterialTheme.typography.titleLarge)
            Text(vm.startError, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
            Button(onClick = { vm.start() }) { Text("重试") }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun MainScreen(vm: AppViewModel, snackbar: SnackbarHostState) {
    var tab by rememberSaveable { mutableStateOf(Tab.Overview) }
    var qr by remember { mutableStateOf<QrTarget?>(null) }

    // 其他标签按返回键先回到概览，概览再按返回键退到后台（见 MainActivity）
    BackHandler(enabled = tab != Tab.Overview) { tab = Tab.Overview }

    // 界面可见时定时刷新状态；有账号在运行或导入时刷得快一些
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    LaunchedEffect(lifecycle) {
        lifecycle.repeatOnLifecycle(Lifecycle.State.STARTED) {
            while (true) {
                vm.refreshStatus()
                val active = vm.status?.accounts?.any { it.running || it.busy.isNotEmpty() || it.fav?.running == true }
                delay(if (active == true) 2000 else 5000)
            }
        }
    }

    // 系统的文件选择器：导出收藏夹、导入收藏夹、添加动态图片
    val saveFav = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("application/json")) {
        vm.saveExport(it)
    }
    val openFav = rememberLauncherForActivityResult(ActivityResultContracts.OpenDocument()) { vm.pickedImport(it) }
    val pickImages = rememberLauncherForActivityResult(ActivityResultContracts.GetMultipleContents()) {
        vm.uploadImages(it)
    }
    val exportFav: (Account) -> Unit = { a -> vm.exportFavorites(a) { name -> saveFav.launch(name) } }
    val importFav: (Account) -> Unit = { a ->
        vm.beginImport(a)
        // 有的文件管理器把 .json 标成 octet-stream 或纯文本
        openFav.launch(arrayOf("application/json", "application/octet-stream", "text/plain"))
    }
    val openQr: (QrTarget) -> Unit = { qr = it }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(tab.label) },
                actions = {
                    if ((tab == Tab.Overview || tab == Tab.Accounts) && vm.status?.accounts?.isNotEmpty() == true) {
                        IconButton(onClick = { vm.runAll() }, enabled = !vm.runningAll && !vm.stopped) {
                            if (vm.runningAll) {
                                CircularProgressIndicator(Modifier.padding(12.dp), strokeWidth = 2.dp)
                            } else {
                                Icon(Icons.Filled.PlayArrow, contentDescription = "全部立即运行")
                            }
                        }
                    }
                },
            )
        },
        bottomBar = {
            NavigationBar {
                Tab.entries.forEach { t ->
                    NavigationBarItem(
                        selected = tab == t,
                        onClick = { tab = t },
                        icon = { Icon(t.icon, contentDescription = null) },
                        label = { Text(t.label) },
                    )
                }
            }
        },
        floatingActionButton = {
            if (tab == Tab.Accounts && vm.status?.accounts?.isNotEmpty() == true && !vm.stopped) {
                ExtendedFloatingActionButton(
                    onClick = { qr = QrTarget(null, "") },
                    icon = { Icon(Icons.Filled.Add, contentDescription = null) },
                    text = { Text("添加账号") },
                )
            }
        },
        snackbarHost = { SnackbarHost(snackbar) },
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding).consumeWindowInsets(padding)) {
            if (vm.stopped) StoppedBanner(vm)
            Box(Modifier.weight(1f)) {
                val s = vm.status
                when {
                    s == null && vm.statusError.isNotEmpty() -> StatusError(vm)
                    s == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        if (!vm.stopped) CircularProgressIndicator()
                    }
                    else -> when (tab) {
                        Tab.Overview -> OverviewScreen(vm, s, openQr) { tab = Tab.Accounts }
                        Tab.Accounts -> AccountsScreen(vm, s, openQr, exportFav, importFav)
                        Tab.Prefs -> SettingsScreen(vm, s) { pickImages.launch("image/*") }
                        Tab.Logs -> LogsScreen(vm)
                    }
                }
            }
        }
    }

    qr?.let { target -> QrDialog(vm, target) { qr = null } }

    vm.favConfirm?.let { f ->
        val minutes = max(1, (f.videos * 2.5 / 60).roundToInt())
        val invalid = if (f.invalid > 0) "（另有 ${f.invalid} 个已失效，会跳过）" else ""
        ConfirmDialog(
            title = "导入到「${f.targetLabel}」？",
            text = "来自「${f.uname.ifEmpty { f.mid }}」的 ${f.folders} 个收藏夹、${f.videos} 个视频$invalid。" +
                "同名的收藏夹会合并，没有的会新建；已经收藏的视频跳过。" +
                "为了不被 B 站限制，每个视频间隔两三秒，大约要 $minutes 分钟，期间这个账号不会运行定时任务。",
            confirm = "开始导入",
            onConfirm = { vm.confirmImport(f) },
            onDismiss = { vm.favConfirm = null },
        )
    }

    if (vm.needPassword) PasswordDialog(vm)
}

/** 本机运行已经停止时显示在最上面 */
@Composable
private fun StoppedBanner(vm: AppViewModel) {
    Surface(color = MaterialTheme.colorScheme.errorContainer, contentColor = MaterialTheme.colorScheme.onErrorContainer) {
        Row(
            Modifier.fillMaxWidth().padding(start = 16.dp, end = 8.dp, top = 6.dp, bottom = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text("本机运行已停止，定时任务不会运行。", Modifier.weight(1f), style = MaterialTheme.typography.bodyMedium)
            TextButton(onClick = { vm.restartLocal() }, enabled = !vm.restarting) {
                if (vm.restarting) ButtonSpinner()
                Text("重新开始")
            }
        }
    }
}

@Composable
private fun StatusError(vm: AppViewModel) {
    Column(
        Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp, Alignment.CenterVertically),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("连不上", style = MaterialTheme.typography.titleMedium)
        Text(vm.statusError, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
        Row {
            Button(onClick = { vm.refresh() }) { Text("重试") }
            Spacer(Modifier.width(8.dp))
            TextButton(onClick = { vm.disconnect() }) { Text("回到首页") }
        }
    }
}

/** 服务器要求重新输入面板密码（重启过或者改了密码） */
@Composable
private fun PasswordDialog(vm: AppViewModel) {
    var password by remember { mutableStateOf("") }
    var keep by remember { mutableStateOf(vm.appState.remember) }
    val submit = { if (password.isNotEmpty() && !vm.reconnecting) vm.reconnect(password, keep) }
    AlertDialog(
        onDismissRequest = {},
        title = { Text("重新输入面板密码") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Muted("服务器 ${vm.appState.server} 要求重新登录，可能是重启过或者改了密码。")
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text("面板密码") },
                    singleLine = true,
                    enabled = !vm.reconnecting,
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
                    keyboardActions = KeyboardActions(onGo = { submit() }),
                    modifier = Modifier.fillMaxWidth(),
                )
                CheckRow("记住密码", keep, enabled = !vm.reconnecting) { keep = it }
                if (vm.passwordError.isNotEmpty()) {
                    Text(vm.passwordError, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { submit() }, enabled = password.isNotEmpty() && !vm.reconnecting) {
                if (vm.reconnecting) ButtonSpinner()
                Text("连接")
            }
        },
        dismissButton = { TextButton(onClick = { vm.disconnect() }, enabled = !vm.reconnecting) { Text("回到首页") } },
    )
}
