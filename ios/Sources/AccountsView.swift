import SwiftUI
import UniformTypeIdentifiers

/// 导出的收藏夹文件，交给系统的「存储到文件」
struct FavDocument: FileDocument {
    static var readableContentTypes: [UTType] { [.json] }

    var data: Data

    init(data: Data) { self.data = data }

    init(configuration: ReadConfiguration) throws {
        data = configuration.file.regularFileContents ?? Data()
    }

    func fileWrapper(configuration: WriteConfiguration) throws -> FileWrapper {
        FileWrapper(regularFileWithContents: data)
    }
}

/// 选好了要导入的文件，等用户确认
struct PendingImport: Identifiable {
    let id = UUID()
    let target: Account
    let data: Data
    let message: String

    init(target: Account, data: Data) throws {
        guard let f = try? JSONDecoder().decode(FavFile.self, from: data),
              f.format == FavFile.formatName, f.version == 1
        else { throw BridgeError("不是 bili-ip-anchor 导出的收藏夹文件") }
        let folders = f.folders ?? []
        var videos = 0
        var invalid = 0
        for folder in folders {
            for v in folder.videos ?? [] {
                if v.invalid == true { invalid += 1 } else { videos += 1 }
            }
        }
        let from = (f.uname ?? "").isEmpty ? (f.mid ?? "") : f.uname!
        let minutes = max(1, Int((Double(videos) * 2.5 / 60).rounded()))
        let skipped = invalid > 0 ? "（另有 \(invalid) 个已失效，会跳过）" : ""
        self.target = target
        self.data = data
        message = "来自「\(from)」的 \(folders.count) 个收藏夹、\(videos) 个视频\(skipped)。"
            + "同名的收藏夹会合并，没有的会新建；已经收藏的视频跳过。"
            + "为了不被 B 站限制，每个视频间隔两三秒，大约要 \(minutes) 分钟，期间这个账号不会运行定时任务。"
    }
}

private let maxFavFile = 20 << 20 // 和 handleFavImport 的上限一样

/// 账号：每个账号一组
struct AccountsView: View {
    @EnvironmentObject var model: AppModel
    @State private var exportDoc: FavDocument?
    @State private var exportName = ""
    @State private var showExporter = false
    @State private var importTarget: Account?
    @State private var showImporter = false
    @State private var pendingImport: PendingImport?

    var body: some View {
        NavigationStack {
            List {
                BannerSection()
                if let s = model.status {
                    if s.accountList.isEmpty {
                        EmptyAccountsSection()
                    }
                    ForEach(s.accountList) { a in
                        AccountSection(
                            account: a,
                            zone: s.zone,
                            export: { await export(a) },
                            importFile: {
                                importTarget = a
                                showImporter = true
                            }
                        )
                    }
                } else if model.statusError == nil {
                    LoadingRow()
                }
            }
            .refreshable { await model.refresh() }
            .navigationTitle("账号")
            .toolbar {
                ToolbarItem(placement: .primaryAction) {
                    Button {
                        model.qr = .add
                    } label: {
                        Label("添加账号", systemImage: "plus")
                    }
                    .disabled(model.stopped)
                }
            }
            .fileExporter(isPresented: $showExporter, document: exportDoc, contentType: .json, defaultFilename: exportName) { result in
                exportDoc = nil
                switch result {
                case .success: model.notify("收藏夹已导出")
                case .failure(let e): model.report(e, title: "保存失败")
                }
            }
            .fileImporter(isPresented: $showImporter, allowedContentTypes: [.json]) { result in
                picked(result)
            }
            .alert(item: $pendingImport) { p in
                Alert(
                    title: Text("导入到「\(p.target.label)」？"),
                    message: Text(p.message),
                    primaryButton: .default(Text("开始导入")) { Task { await startImport(p) } },
                    secondaryButton: .cancel(Text("取消"))
                )
            }
        }
    }

    @MainActor
    private func export(_ a: Account) async {
        do {
            let r = try await model.send("GET", "/api/accounts/\(a.mid)/favorites")
            var name = r.filename.isEmpty ? "bili-favorites-\(a.mid).json" : r.filename
            // 系统会按 contentType 加上扩展名
            if name.hasSuffix(".json") { name.removeLast(5) }
            exportName = name
            exportDoc = FavDocument(data: r.body)
            showExporter = true
        } catch {
            model.report(error, title: "导出失败")
        }
    }

    @MainActor
    private func picked(_ result: Result<URL, Error>) {
        guard let target = importTarget else { return }
        importTarget = nil
        switch result {
        case .failure(let e):
            model.report(e, title: "读不了这个文件")
        case .success(let url):
            let scoped = url.startAccessingSecurityScopedResource()
            defer { if scoped { url.stopAccessingSecurityScopedResource() } }
            do {
                let size = (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0
                if size > maxFavFile { throw BridgeError("文件太大，不像是导出的收藏夹") }
                let data = try Data(contentsOf: url)
                pendingImport = try PendingImport(target: target, data: data)
            } catch {
                model.report(error, title: "不能导入")
            }
        }
    }

    @MainActor
    private func startImport(_ p: PendingImport) async {
        do {
            let r: FavImportStarted = try await model.request("POST", "/api/accounts/\(p.target.mid)/favorites", p.data)
            model.notify("开始导入 \(r.total) 个视频，进度显示在账号下面")
        } catch {
            model.report(error, title: "没有开始导入")
        }
        await model.refresh()
    }
}

/// 一个账号：资料、操作、属地、运行、Cookie、收藏夹
struct AccountSection: View {
    @EnvironmentObject var model: AppModel
    let account: Account
    let zone: Zone?
    let export: () async -> Void
    let importFile: () -> Void

    @State private var confirmLogout = false
    @State private var confirmUnauth = false

    var body: some View {
        let a = account
        let busy = a.isBusy
        Section {
            HStack(spacing: 12) {
                Avatar(account: a, size: 48)
                VStack(alignment: .leading, spacing: 6) {
                    HStack(spacing: 8) {
                        Text(a.label).font(.headline).lineLimit(1)
                        StateTag(account: a)
                    }
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 6) {
                            Pill("UID \(a.mid)")
                            if let st = a.stats {
                                Pill("Lv\(st.level)", color: st.level >= 6 ? .red : (st.level == 5 ? .orange : .secondary))
                                if let vip = st.vip, !vip.isEmpty { Pill(vip, color: .brand) }
                                Pill("硬币 \(Fmt.number(st.coins))")
                                if st.bcoins > 0 { Pill("B币 \(Fmt.number(st.bcoins))") }
                            }
                        }
                    }
                }
            }
            .padding(.vertical, 4)

            if a.loggedIn {
                AsyncButton {
                    await model.act(a.mid, "POST", "run", done: nil, failTitle: "没有运行")
                } label: {
                    Label(a.running ? "运行中" : "立即运行", systemImage: "play.fill")
                }
                .disabled(busy)
            } else {
                Button {
                    model.qr = .add
                } label: {
                    Label("重新扫码", systemImage: "qrcode.viewfinder")
                }
            }
            AsyncButton {
                await model.act(a.mid, "POST", "check", done: "资料已刷新", failTitle: "没有刷新")
            } label: {
                Label("刷新资料", systemImage: "arrow.clockwise")
            }
            .disabled(busy || !a.loggedIn)

            homeRow(a, busy: busy)
            targetRow
            runRow(a)
            cookieRow(a, busy: busy)
            favoritesRow(a, busy: busy)

            Button(role: .destructive) {
                confirmLogout = true
            } label: {
                Label("退出账号", systemImage: "rectangle.portrait.and.arrow.right")
            }
            .confirmationDialog("退出「\(a.label)」？", isPresented: $confirmLogout, titleVisibility: .visible) {
                Button("退出", role: .destructive) {
                    Task { await model.act(a.mid, "DELETE", "", done: "已退出 \(a.label)", failTitle: "没有退出") }
                }
            } message: {
                Text("这份会话会在 B 站注销并删除，之后不再为这个账号运行。")
            }
        } footer: {
            if let e = accountError(a) {
                Text(e).foregroundColor(.red)
            }
        }
    }

    private func homeRow(_ a: Account, busy: Bool) -> some View {
        let h = homeSummary(a, zone: zone)
        let badge: (String, Color)? = h.badge.map { ($0, h.matched == true ? Color.green : Color.orange) }
        return InfoRow(icon: "mappin.and.ellipse", title: "主页属地", value: h.value, pending: h.pending, badge: badge, sub: h.sub) {
            if a.appAuth {
                Button("取消授权") { confirmUnauth = true }
                    .buttonStyle(.borderless)
                    .font(.subheadline)
                    .disabled(busy)
                    .confirmationDialog("取消「\(a.label)」的主页属地授权？", isPresented: $confirmUnauth, titleVisibility: .visible) {
                        Button("取消授权", role: .destructive) {
                            Task { await model.act(a.mid, "DELETE", "appauth", done: "已取消授权", failTitle: "没有取消授权") }
                        }
                    } message: {
                        Text("会在 B 站撤销这份 App 授权，之后不再显示主页属地。不影响定时任务。")
                    }
            } else {
                Button("授权查询") { model.qr = QRTarget(mid: a.mid, label: a.label) }
                    .buttonStyle(.bordered)
                    .font(.subheadline)
                    .disabled(busy || !a.loggedIn)
            }
        }
    }

    private var targetRow: some View {
        InfoRow(
            icon: "globe.asia.australia",
            title: "目标属地",
            value: zone.map { $0.region.isEmpty ? "未知" : $0.region } ?? "查询中",
            pending: zone == nil,
            sub: zone.map { [$0.addr, $0.isp].filter { !$0.isEmpty }.joined(separator: " · ") } ?? ""
        )
    }

    private func runRow(_ a: Account) -> some View {
        let r = runSummary(a)
        return InfoRow(icon: "clock", title: "运行", value: r.value, sub: r.lines.joined(separator: " · "))
    }

    private func cookieRow(_ a: Account, busy: Bool) -> some View {
        let c = cookieSummary(a)
        let refreshing = a.busy == "刷新 Cookie"
        return InfoRow(icon: "key", title: "Cookie", value: c.value, valueColor: c.color, sub: c.sub) {
            AsyncButton {
                await model.act(a.mid, "POST", "cookie/refresh", done: "Cookie 已刷新", failTitle: "没有刷新 Cookie")
            } label: {
                Text(refreshing ? "正在刷新" : "刷新")
            }
            .buttonStyle(.bordered)
            .font(.subheadline)
            .disabled(busy || !a.loggedIn || !a.cookie.canRefresh)
        }
    }

    private func favoritesRow(_ a: Account, busy: Bool) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 12) {
                Image(systemName: "star.square.on.square")
                    .foregroundColor(.secondary)
                    .frame(width: 22)
                VStack(alignment: .leading, spacing: 2) {
                    Text("收藏夹").font(.subheadline.weight(.semibold))
                    Text("导出成文件，可以导入到这个或别的账号").font(.caption).foregroundColor(.secondary)
                }
            }
            HStack(spacing: 12) {
                AsyncButton(action: export) {
                    Label("导出", systemImage: "square.and.arrow.up")
                }
                .buttonStyle(.bordered)
                Button(action: importFile) {
                    Label("导入", systemImage: "square.and.arrow.down")
                }
                .buttonStyle(.bordered)
            }
            .disabled(busy || !a.loggedIn)
            if let job = a.favImport {
                ImportProgressView(job: job)
            }
        }
        .padding(.vertical, 4)
    }
}

/// 卡片里的一行信息
struct InfoRow<Trailing: View>: View {
    let icon: String
    let title: String
    let value: String
    let valueColor: Color?
    let pending: Bool
    let badge: (String, Color)?
    let sub: String
    let trailing: Trailing

    init(
        icon: String,
        title: String,
        value: String,
        valueColor: Color? = nil,
        pending: Bool = false,
        badge: (String, Color)? = nil,
        sub: String = "",
        @ViewBuilder trailing: () -> Trailing
    ) {
        self.icon = icon
        self.title = title
        self.value = value
        self.valueColor = valueColor
        self.pending = pending
        self.badge = badge
        self.sub = sub
        self.trailing = trailing()
    }

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: icon)
                .foregroundColor(.secondary)
                .frame(width: 22)
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.caption).foregroundColor(.secondary)
                HStack(spacing: 6) {
                    Text(value)
                        .font(.body.weight(pending ? .regular : .semibold))
                        .foregroundColor(valueColor ?? (pending ? .secondary : .primary))
                    if let badge {
                        Pill(badge.0, color: badge.1)
                    }
                }
                if !sub.isEmpty {
                    Text(sub).font(.caption).foregroundColor(.secondary)
                }
            }
            Spacer(minLength: 0)
            trailing
        }
    }
}

extension InfoRow where Trailing == EmptyView {
    init(
        icon: String,
        title: String,
        value: String,
        valueColor: Color? = nil,
        pending: Bool = false,
        badge: (String, Color)? = nil,
        sub: String = ""
    ) {
        self.init(icon: icon, title: title, value: value, valueColor: valueColor, pending: pending, badge: badge, sub: sub) {
            EmptyView()
        }
    }
}

/// 导入收藏夹的进度
struct ImportProgressView: View {
    let job: FavImport

    var body: some View {
        let failed = !(job.error ?? "").isEmpty
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(head)
                    .font(.footnote.weight(.medium))
                    .foregroundColor(failed ? .red : .primary)
                Spacer()
                Text(progressText)
                    .font(.footnote.monospacedDigit())
                    .foregroundColor(.secondary)
            }
            ProgressView(value: Double(job.done), total: Double(max(job.total, 1)))
                .tint(failed ? .red : .accentColor)
            Text(counts).font(.caption).foregroundColor(.secondary)
            if let e = job.error, !e.isEmpty {
                Text(e).font(.caption).foregroundColor(.red)
            }
            if let failures = job.failures, !failures.isEmpty {
                DisclosureGroup("没有收藏上的视频（\(failures.count)）") {
                    ForEach(failures, id: \.bvid) { f in
                        VStack(alignment: .leading, spacing: 2) {
                            if let u = Links.video(f.bvid) {
                                Link(f.bvid, destination: u).font(.caption.monospaced())
                            }
                            Text([f.title ?? "", f.reason].filter { !$0.isEmpty }.joined(separator: " — "))
                                .font(.caption)
                                .foregroundColor(.secondary)
                        }
                    }
                }
                .font(.caption)
            }
        }
    }

    private var head: String {
        if job.running {
            if let f = job.folder, !f.isEmpty { return "正在导入「\(f)」" }
            return "正在导入"
        }
        return (job.error ?? "").isEmpty ? "导入完成" : "导入中途停下"
    }

    private var progressText: String {
        var s = "\(job.done)/\(job.total)"
        if let f = job.finished, GoTime.isSet(f) { s += " · \(Fmt.time(f))" }
        return s
    }

    private var counts: String {
        var parts = ["新增 \(job.added)", "已有 \(job.skipped)"]
        if job.failed > 0 { parts.append("失败 \(job.failed)") }
        if let created = job.created, !created.isEmpty {
            parts.append("新建了" + created.map { "「\($0)」" }.joined())
        }
        return parts.joined(separator: " · ")
    }
}
