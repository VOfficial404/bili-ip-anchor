import SwiftUI

extension String {
    var trimmed: String { trimmingCharacters(in: .whitespacesAndNewlines) }
}

/// 测试代理的结果；ok 为 nil 是正在测试或者不需要测试
struct ProbeResult: Equatable {
    var text: String
    var ok: Bool?

    var color: Color {
        switch ok {
        case true?: return .green
        case false?: return .red
        case nil: return .secondary
        }
    }
}

func zoneText(_ z: Zone) -> String {
    let isp = z.isp.isEmpty ? "" : " · \(z.isp)"
    return "B 站看到的是 \(z.place)\(isp) · \(z.addr)，主页属地会变成\(z.region)。"
}

/// 首页：选在手机上运行还是连接服务器
struct SetupView: View {
    @EnvironmentObject var model: AppModel

    enum Mode: Hashable { case local, remote }

    @State private var mode: Mode = .local
    @State private var proxy = ""
    @State private var server = ""
    @State private var password = ""
    @State private var remember = true
    @State private var probe: ProbeResult?
    @State private var probing = false
    @State private var busy = false
    @State private var error: String?
    @State private var loaded = false

    private var canSubmit: Bool {
        mode == .local ? !proxy.trimmed.isEmpty : !server.trimmed.isEmpty && !password.isEmpty
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Picker("用法", selection: $mode) {
                        Text("在手机上运行").tag(Mode.local)
                        Text("连接服务器").tag(Mode.remote)
                    }
                    .pickerStyle(.segmented)
                    .disabled(busy)
                    .onChange(of: mode) { _ in error = nil }
                } footer: {
                    Text("选一种用法。之后可以在「设置」里回到这里换一种。")
                }

                if mode == .local {
                    Section {
                        TextField("http://127.0.0.1:7890", text: $proxy)
                            .keyboardType(.URL)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .font(.body.monospaced())
                            .disabled(busy)
                            .onChange(of: proxy) { _ in probe = nil }
                        Button {
                            Task { await test() }
                        } label: {
                            HStack {
                                Text("测试代理")
                                if probing {
                                    Spacer()
                                    ProgressView()
                                }
                            }
                        }
                        .disabled(probing || busy || proxy.trimmed.isEmpty)
                        if let probe {
                            Text(probe.text).font(.footnote).foregroundColor(probe.color)
                        }
                    } header: {
                        Text("代理")
                    } footer: {
                        Text("没有服务器也可以用：手机定时以账号身份发请求，经过代理出去，B 站看到的是代理的出口 IP。填代理软件的本地端口，支持 http:// 和 socks5://。\n\niOS 不允许 App 一直在后台运行：切到后台后定时任务暂停，回到 App 时补上。要不间断运行，请用服务器。")
                    }
                } else {
                    Section {
                        TextField("http://127.0.0.1:8080", text: $server)
                            .keyboardType(.URL)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .textContentType(.URL)
                            .disabled(busy)
                        SecureField("面板密码", text: $password)
                            .textContentType(.password)
                            .submitLabel(.go)
                            .onSubmit { Task { await submit() } }
                            .disabled(busy)
                        Toggle("记住密码，下次打开自动连接", isOn: $remember)
                            .disabled(busy)
                    } header: {
                        Text("服务器")
                    } footer: {
                        Text("定时任务在你的云服务器上跑。填 bili-ip-anchor WebUI 的地址和面板密码（启动时 WEBUI_PASSWORD 里设置的那个）；通过 SSH 隧道访问时，地址是 http://127.0.0.1:8080。")
                    }
                }

                if let error {
                    Section {
                        Text(error).foregroundColor(.red)
                    }
                }

                Section {
                    Button {
                        Task { await submit() }
                    } label: {
                        HStack {
                            Spacer()
                            if busy {
                                ProgressView().padding(.trailing, 4)
                            }
                            Text(mode == .local ? "开始运行" : "连接").bold()
                            Spacer()
                        }
                    }
                    .disabled(!canSubmit || busy)
                }
            }
            .scrollDismissesKeyboard(.interactively)
            .navigationTitle("Bili IP Anchor")
        }
        .onAppear {
            guard !loaded else { return }
            loaded = true
            proxy = model.core.proxy
            server = model.core.server
            mode = model.core.server.isEmpty ? .local : .remote
        }
    }

    @MainActor
    private func test() async {
        let p = proxy.trimmed
        guard !p.isEmpty else { return }
        probing = true
        probe = ProbeResult(text: "正在经过代理访问 B 站…", ok: nil)
        do {
            let z = try await Core.run { try Core.probeProxy(p) }
            probe = ProbeResult(text: "可用。" + zoneText(z), ok: true)
        } catch {
            probe = ProbeResult(text: error.localizedDescription, ok: false)
        }
        probing = false
    }

    @MainActor
    private func submit() async {
        guard canSubmit, !busy else { return }
        busy = true
        error = nil
        do {
            if mode == .local {
                let r = try await model.startLocal(proxy)
                model.enterMain()
                if let w = r.warning, !w.isEmpty {
                    model.alert = AlertMessage(title: "请注意", message: w)
                }
            } else {
                try await model.connect(server: server, password: password, remember: remember)
                model.enterMain()
            }
        } catch {
            self.error = error.localizedDescription
        }
        busy = false
    }
}
