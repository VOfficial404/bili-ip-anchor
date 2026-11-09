package io.github.vofficial404.biliipanchor

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.LocalTextStyle
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp

/** 首页：选在手机上运行还是连接服务器 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalLayoutApi::class)
@Composable
fun SetupScreen(vm: AppViewModel, snackbar: SnackbarHostState) {
    val st = vm.appState
    var local by rememberSaveable { mutableStateOf(st.server.isEmpty()) }
    var proxy by rememberSaveable { mutableStateOf(st.proxy) }
    var server by rememberSaveable { mutableStateOf(st.server) }
    var password by rememberSaveable { mutableStateOf("") }
    var keep by rememberSaveable { mutableStateOf(true) }
    val busy = vm.setupBusy

    val canSubmit = if (local) proxy.isNotBlank() else server.isNotBlank() && password.isNotEmpty()
    val submit = {
        if (canSubmit && !busy) {
            if (local) vm.startLocal(proxy) else vm.connect(server, password, keep)
        }
    }

    Scaffold(snackbarHost = { SnackbarHost(snackbar) }) { padding ->
        Box(
            Modifier
                .fillMaxSize()
                .padding(padding)
                .consumeWindowInsets(padding)
                .imePadding()
                .verticalScroll(rememberScrollState()),
            contentAlignment = Alignment.TopCenter,
        ) {
            Column(
                Modifier.widthIn(max = 560.dp).fillMaxWidth().padding(horizontal = 20.dp, vertical = 32.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Text("Bili IP Anchor", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.SemiBold)
                Text(
                    "选一种用法。之后可以在「设置」里回到这里换一种。",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )

                SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                    SegmentedButton(
                        selected = local,
                        onClick = {
                            local = true
                            vm.setupError = ""
                        },
                        shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
                        enabled = !busy,
                    ) { Text("在手机上运行") }
                    SegmentedButton(
                        selected = !local,
                        onClick = {
                            local = false
                            vm.setupError = ""
                        },
                        shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
                        enabled = !busy,
                    ) { Text("连接服务器") }
                }

                if (local) {
                    Muted(
                        "没有服务器也可以用：手机定时以账号身份发请求，经过代理出去，B 站看到的是代理的出口 IP。" +
                            "看视频不经过它。退出界面后在通知栏里继续运行。",
                    )
                    Row(verticalAlignment = Alignment.Top) {
                        OutlinedTextField(
                            value = proxy,
                            onValueChange = {
                                proxy = it
                                vm.setupProbe = null
                            },
                            label = { Text("代理") },
                            placeholder = { Text("http://127.0.0.1:7890") },
                            supportingText = { Text("代理软件的本地端口，支持 http:// 和 socks5://。") },
                            singleLine = true,
                            enabled = !busy,
                            textStyle = LocalTextStyle.current.copy(fontFamily = FontFamily.Monospace),
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Go),
                            keyboardActions = KeyboardActions(onGo = { submit() }),
                            modifier = Modifier.weight(1f),
                        )
                        OutlinedButton(
                            onClick = { vm.probeSetup(proxy) },
                            enabled = !busy,
                            modifier = Modifier.padding(start = 8.dp, top = 8.dp),
                        ) { Text("测试") }
                    }
                    vm.setupProbe?.let { ProbeText(it) }
                } else {
                    Muted(
                        "定时任务在你的云服务器上跑。填 bili-ip-anchor WebUI 的地址和面板密码；" +
                            "通过 SSH 隧道访问时，地址是 http://127.0.0.1:8080。",
                    )
                    OutlinedTextField(
                        value = server,
                        onValueChange = { server = it },
                        label = { Text("服务器地址") },
                        placeholder = { Text("http://127.0.0.1:8080") },
                        singleLine = true,
                        enabled = !busy,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Next),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = password,
                        onValueChange = { password = it },
                        label = { Text("面板密码") },
                        supportingText = { Text("启动服务器时 WEBUI_PASSWORD 里设置的那个。") },
                        singleLine = true,
                        enabled = !busy,
                        visualTransformation = PasswordVisualTransformation(),
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
                        keyboardActions = KeyboardActions(onGo = { submit() }),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    CheckRow("记住密码，下次打开自动连接", keep, enabled = !busy) { keep = it }
                }

                if (vm.setupError.isNotEmpty()) {
                    Text(vm.setupError, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
                }

                Button(onClick = { submit() }, enabled = canSubmit && !busy, modifier = Modifier.fillMaxWidth()) {
                    val label = if (local) "开始运行" else "连接"
                    if (busy && vm.setupProbe?.ok != null || busy && vm.setupProbe == null) {
                        ButtonSpinner()
                        Text("正在$label")
                    } else {
                        Text(label)
                    }
                }
            }
        }
    }
}

/** 测试代理的结果 */
@Composable
fun ProbeText(p: Probe) {
    val color = when (p.ok) {
        true -> LocalExtraColors.current.success
        false -> MaterialTheme.colorScheme.error
        null -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    Text(p.text, style = MaterialTheme.typography.bodySmall, color = color)
}
