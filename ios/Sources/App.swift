import SwiftUI

/// 原生 SwiftUI 界面。Go 核心（mobile/mobile.go）负责定时任务和服务器的连接，界面通过 Core.call 调用面板接口。
/// iOS 不允许 App 在后台一直运行：切到后台后本机的定时任务暂停，回到前台时按墙上时间补上。
@main
struct BiliIPAnchorApp: App {
    @StateObject private var model = AppModel()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(model)
                .task { await model.start() }
        }
        .onChange(of: scenePhase) { phase in
            if phase == .active {
                Task { await model.becameActive() }
            }
        }
    }
}

/// 按阶段显示：启动中、启动失败、首页（选用法）、主界面
struct RootView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        ZStack(alignment: .bottom) {
            switch model.phase {
            case .loading:
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            case .failed(let message):
                VStack(spacing: 12) {
                    Image(systemName: "exclamationmark.triangle")
                        .font(.largeTitle)
                        .foregroundColor(.orange)
                    Text("启动失败").font(.headline)
                    Text(message)
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                        .multilineTextAlignment(.center)
                    Button("重试") { model.retryStart() }
                        .buttonStyle(.borderedProminent)
                }
                .padding(24)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            case .setup:
                SetupView()
            case .main:
                MainView()
            }

            if let notice = model.notice {
                NoticeView(text: notice)
                    .padding(.bottom, 72)
            }
        }
        .alert(item: $model.alert) { a in
            Alert(title: Text(a.title), message: Text(a.message), dismissButton: .default(Text("好")))
        }
    }
}

/// 主界面：底部四个标签
struct MainView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        TabView(selection: $model.tab) {
            OverviewView()
                .tabItem { Label("概览", systemImage: "square.grid.2x2") }
                .tag(MainTab.overview)
            AccountsView()
                .tabItem { Label("账号", systemImage: "person.2") }
                .tag(MainTab.accounts)
            SettingsView()
                .tabItem { Label("设置", systemImage: "gearshape") }
                .tag(MainTab.settings)
            LogsView()
                .tabItem { Label("日志", systemImage: "doc.plaintext") }
                .tag(MainTab.logs)
        }
        // 界面显示时定时刷新状态；有账号在运行或导入时刷得快一些
        .task {
            while !Task.isCancelled {
                await model.refresh()
                let active = model.status?.accountList.contains { $0.isBusy || $0.favImport?.running == true } ?? false
                try? await Task.sleep(nanoseconds: active ? 2_000_000_000 : 5_000_000_000)
            }
        }
        .sheet(item: $model.qr) { target in
            QRView(target: target)
                .environmentObject(model)
        }
        .sheet(isPresented: $model.needsPassword) {
            PasswordSheet()
                .environmentObject(model)
                .interactiveDismissDisabled()
        }
    }
}

/// 服务器要求重新输入面板密码（重启过或者改了密码）
struct PasswordSheet: View {
    @EnvironmentObject var model: AppModel
    @State private var password = ""
    @State private var remember = true
    @State private var error: String?
    @State private var busy = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    SecureField("面板密码", text: $password)
                        .textContentType(.password)
                        .submitLabel(.go)
                        .onSubmit { Task { await submit() } }
                    Toggle("记住密码", isOn: $remember)
                } footer: {
                    Text("服务器 \(model.core.server) 要求重新登录，可能是重启过或者改了密码。")
                }
                if let error {
                    Section {
                        Text(error).foregroundColor(.red)
                    }
                }
            }
            .navigationTitle("重新输入密码")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("回到首页") { Task { await model.disconnect() } }
                        .disabled(busy)
                }
                ToolbarItem(placement: .confirmationAction) {
                    if busy {
                        ProgressView()
                    } else {
                        Button("连接") { Task { await submit() } }
                            .disabled(password.isEmpty)
                    }
                }
            }
        }
        .onAppear { remember = model.core.remember }
    }

    @MainActor
    private func submit() async {
        guard !password.isEmpty, !busy else { return }
        busy = true
        error = nil
        do {
            try await model.connect(server: model.core.server, password: password, remember: remember)
            model.needsPassword = false
            await model.refresh()
        } catch {
            self.error = error.localizedDescription
        }
        busy = false
    }
}
