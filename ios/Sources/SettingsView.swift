import PhotosUI
import SwiftUI
import UIKit

/// 设置表单，保存时整个发给 PUT /api/settings
struct SettingsForm: Equatable {
    var videos: [String] = []
    var doDynamic = false
    var dynamicText = ""
    var interval = ""
    var jitter = ""
    var proxy = ""
    var logLevel = "info"

    init() {}

    init(_ s: PanelSettings) {
        videos = s.videos ?? []
        doDynamic = s.doDynamic
        dynamicText = s.dynamicText ?? ""
        interval = s.interval
        jitter = s.jitter
        proxy = s.proxy ?? ""
        logLevel = (s.logLevel ?? "").isEmpty ? "info" : s.logLevel!
    }

    var body: SettingsBody {
        SettingsBody(
            videos: videos,
            doDynamic: doDynamic,
            dynamicText: dynamicText.trimmed,
            interval: interval.trimmed,
            jitter: jitter.trimmed,
            proxy: proxy.trimmed,
            logLevel: logLevel
        )
    }
}

private let levels: [(value: String, label: String, hint: String)] = [
    ("debug", "调试（debug）", "记录所有日志，包括每一个发给 B 站的请求，排查问题时再打开"),
    ("info", "信息（info）", "每轮运行、登录、设置变更等常规记录，推荐"),
    ("warn", "警告（warn）", "只记录警告和错误"),
    ("error", "错误（error）", "只记录错误"),
]

private let maxImages = 9
private let maxImageSize = 10 << 20 // 和 core 的 maxImageSize 一样
private let maxVideos = 20
private let bvPattern = try! NSRegularExpression(pattern: "BV[0-9A-Za-z]{10}")

/// 设置：所有账号共用，保存后立即生效。动态图片上传、删除时立即生效。
struct SettingsView: View {
    @EnvironmentObject var model: AppModel
    @State private var form = SettingsForm()
    @State private var base: SettingsForm?
    @State private var saving = false
    @State private var formError: String?
    @State private var newVideo = ""
    @State private var videoError: String?
    @State private var adding = false
    @State private var probe: ProbeResult?
    @State private var testing = false
    @State private var photos: [PhotosPickerItem] = []
    @State private var uploading = false
    @State private var deleting = ""
    @State private var confirmStop = false
    @State private var confirmBack = false

    private var edited: Bool { base != nil && form != base }

    var body: some View {
        NavigationStack {
            Group {
                if let s = model.status, base != nil {
                    content(s)
                } else {
                    List {
                        BannerSection()
                        if model.statusError == nil { LoadingRow() }
                    }
                }
            }
            .navigationTitle("设置")
            .toolbar {
                if edited {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("撤销") {
                            if let base { form = base }
                            formError = nil
                            videoError = nil
                            probe = nil
                        }
                        .disabled(saving)
                    }
                    ToolbarItem(placement: .confirmationAction) {
                        if saving {
                            ProgressView()
                        } else {
                            Button("保存") { Task { await save() } }.bold()
                        }
                    }
                }
            }
        }
        .onAppear { sync() }
        .onChange(of: model.status?.settings) { _ in sync() }
        .onChange(of: photos) { items in
            Task { await upload(items) }
        }
    }

    /// 跟着服务端的设置走；改过还没保存时不覆盖表单
    private func sync() {
        guard let s = model.status?.settings else { return }
        let f = SettingsForm(s)
        if !edited { form = f }
        base = f
    }

    private func content(_ s: Status) -> some View {
        Form {
            BannerSection()
            if let formError {
                Section {
                    Text(formError).foregroundColor(.red)
                }
            }
            scheduleSection
            videosSection
            dynamicSection(s)
            proxySection
            connectionSection(s)
        }
        .scrollDismissesKeyboard(.interactively)
    }

    // MARK: 运行

    private var scheduleSection: some View {
        Section {
            LabeledContent("每轮间隔") {
                TextField("6h", text: $form.interval)
                    .multilineTextAlignment(.trailing)
                    .keyboardType(.asciiCapable)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
            }
            LabeledContent("随机抖动") {
                TextField("1h", text: $form.jitter)
                    .multilineTextAlignment(.trailing)
                    .keyboardType(.asciiCapable)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
            }
            Picker("日志级别", selection: $form.logLevel) {
                ForEach(levels, id: \.value) { l in
                    Text(l.label).tag(l.value)
                }
            }
        } header: {
            Text("运行")
        } footer: {
            let hint = levels.first { $0.value == form.logLevel }?.hint ?? ""
            Text("间隔写成 90m、6h、12h 这样，最少 10m；抖动是每轮额外随机等 0 到这么久，让时间不那么规律。\n日志级别：\(hint)。调试级别会把每一个 B 站请求都记进日志，日志很快会被刷满。")
        }
    }

    // MARK: 心跳视频

    private var videosSection: some View {
        Section {
            if form.videos.isEmpty {
                Text("还没有视频，至少添加一个。").foregroundColor(.secondary)
            }
            ForEach(form.videos, id: \.self) { bvid in
                VideoRow(bvid: bvid)
                    .deleteDisabled(form.videos.count <= 1)
            }
            .onDelete { offsets in
                guard form.videos.count - offsets.count >= 1 else { return }
                form.videos.remove(atOffsets: offsets)
            }
            HStack {
                TextField("BV 号或视频链接", text: $newVideo)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .submitLabel(.done)
                    .onSubmit { Task { await addVideos() } }
                    .onChange(of: newVideo) { _ in videoError = nil }
                if adding {
                    ProgressView()
                } else {
                    Button("添加") { Task { await addVideos() } }
                        .buttonStyle(.borderless)
                        .disabled(newVideo.trimmed.isEmpty)
                }
            }
            if let videoError {
                Text(videoError).font(.footnote).foregroundColor(.red)
            }
        } header: {
            Text("心跳视频")
        } footer: {
            Text("每轮从中随机选一个视频上报观看心跳。任意公开视频都可以，最多 \(maxVideos) 个。左滑可以移除。")
        }
    }

    /// 从粘贴的文字里找出 BV 号，查询确认视频存在后加进表单
    @MainActor
    private func addVideos() async {
        let text = newVideo
        let ns = text as NSString
        var found: [String] = []
        for m in bvPattern.matches(in: text, range: NSRange(location: 0, length: ns.length)) {
            let bv = ns.substring(with: m.range)
            if !found.contains(bv) { found.append(bv) }
        }
        guard !found.isEmpty else {
            videoError = "没找到 BV 号。粘贴视频链接或 BV 开头的 12 位编号"
            return
        }
        adding = true
        videoError = nil
        var ok: [String] = []
        var bad: [String] = []
        for bvid in found where !form.videos.contains(bvid) && !ok.contains(bvid) {
            if form.videos.count + ok.count >= maxVideos {
                bad.append("最多 \(maxVideos) 个视频")
                break
            }
            do {
                _ = try await model.lookupVideo(bvid)
                ok.append(bvid)
            } catch {
                if model.isUnauthorized(error) { break }
                bad.append("\(bvid)：\(error.localizedDescription)")
            }
        }
        adding = false
        newVideo = ""
        form.videos += ok
        if !bad.isEmpty { videoError = bad.joined(separator: "；") }
    }

    // MARK: 动态

    private func dynamicSection(_ s: Status) -> some View {
        Section {
            Toggle("发动态", isOn: $form.doDynamic)
            TextField("文字（可以不写）", text: $form.dynamicText, axis: .vertical)
                .lineLimit(3...8)
            if !s.imageList.isEmpty {
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 84), spacing: 8)], spacing: 8) {
                    ForEach(s.imageList) { img in
                        imageTile(img.name)
                    }
                }
                .padding(.vertical, 4)
            }
            PhotosPicker(
                selection: $photos,
                maxSelectionCount: max(1, maxImages - s.imageList.count),
                matching: .images
            ) {
                HStack {
                    Label(uploading ? "正在上传" : "添加图片", systemImage: "photo.badge.plus")
                    if uploading {
                        Spacer()
                        ProgressView()
                    }
                }
            }
            .disabled(uploading || s.imageList.count >= maxImages)
            Text(dynamicPreview(s.imageList.count))
                .font(.subheadline)
                .foregroundColor(.secondary)
        } header: {
            Text("动态")
        } footer: {
            Text("每轮发一条动态，几秒后删掉。只发心跳不能改变属地时再打开；动态会短暂出现在粉丝的动态流里。图片最多 \(maxImages) 张，每张不超过 10 MB，上传和删除立即生效，不需要保存。")
        }
    }

    private func imageTile(_ name: String) -> some View {
        ZStack(alignment: .topTrailing) {
            LoadedImage(.panel("/api/dynamic/images/\(name)")) {
                Color.secondary.opacity(0.15)
            }
            .frame(minWidth: 0, maxWidth: .infinity)
            .frame(height: 84)
            .clipped()
            .clipShape(RoundedRectangle(cornerRadius: 8))
            Button {
                Task { await deleteImage(name) }
            } label: {
                if deleting == name {
                    ProgressView().padding(4)
                } else {
                    Image(systemName: "xmark.circle.fill")
                        .symbolRenderingMode(.palette)
                        .foregroundStyle(Color.white, Color.black.opacity(0.6))
                        .font(.title3)
                }
            }
            .buttonStyle(.borderless)
            .disabled(!deleting.isEmpty)
            .padding(4)
            .accessibilityLabel("删除这张图片")
        }
    }

    private func dynamicPreview(_ images: Int) -> String {
        if !form.doDynamic { return "不发动态，只上报观看心跳。" }
        let text = !form.dynamicText.trimmed.isEmpty
        switch (text, images > 0) {
        case (true, true): return "每轮发一条带文字和 \(images) 张图片的动态。"
        case (true, false): return "每轮发一条纯文字动态。"
        case (false, true): return "每轮发一条带 \(images) 张图片的动态。"
        case (false, false): return "没有文字和图片，每轮发内置的一张图片。"
        }
    }

    @MainActor
    private func upload(_ items: [PhotosPickerItem]) async {
        guard !items.isEmpty, !uploading else { return }
        uploading = true
        var n = 0
        for item in items {
            do {
                guard let raw = try await item.loadTransferable(type: Data.self),
                      let data = uploadData(raw)
                else { throw BridgeError("读不了这张图片") }
                if data.count > maxImageSize { throw BridgeError("图片不能超过 10 MB") }
                try await model.send("POST", "/api/dynamic/images", data)
                n += 1
            } catch {
                model.report(error, title: "上传失败")
                break
            }
        }
        photos = []
        uploading = false
        if n > 0 { model.notify("已添加 \(n) 张图片") }
        await model.refresh()
    }

    @MainActor
    private func deleteImage(_ name: String) async {
        deleting = name
        do {
            try await model.send("DELETE", "/api/dynamic/images/\(name)")
        } catch {
            model.report(error, title: "没有删除")
        }
        await model.refresh()
        deleting = ""
    }

    // MARK: 代理

    private var proxySection: some View {
        Section {
            TextField(model.core.isLocal ? "http://127.0.0.1:7890" : "留空表示直连", text: $form.proxy)
                .keyboardType(.URL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
                .font(.body.monospaced())
                .onChange(of: form.proxy) { _ in probe = nil }
            Button {
                Task { await testProxy() }
            } label: {
                HStack {
                    Text("测试代理")
                    if testing {
                        Spacer()
                        ProgressView()
                    }
                }
            }
            .disabled(testing)
            if let probe {
                Text(probe.text).font(.footnote).foregroundColor(probe.color)
            }
            if model.core.isLocal && form.proxy.trimmed.isEmpty {
                Text("留空时直接从手机发出，B 站看到的是手机自己的 IP。")
                    .font(.footnote)
                    .foregroundColor(.orange)
            }
        } header: {
            Text("代理")
        } footer: {
            Text("所有以账号身份发出的 B 站请求都经过它，B 站看到的就是代理的出口 IP。支持 http://、socks5://，例如 http://127.0.0.1:7890。")
        }
    }

    @MainActor
    private func testProxy() async {
        let p = form.proxy.trimmed
        guard !p.isEmpty else {
            probe = ProbeResult(text: "留空表示直连，不需要测试。", ok: nil)
            return
        }
        testing = true
        probe = ProbeResult(text: "正在经过代理访问 B 站…", ok: nil)
        do {
            let body = try JSONEncoder().encode(["proxy": p])
            let z: Zone = try await model.request("POST", "/api/proxy/test", body)
            probe = ProbeResult(text: "可用。" + zoneText(z), ok: true)
        } catch {
            probe = model.isUnauthorized(error) ? nil : ProbeResult(text: error.localizedDescription, ok: false)
        }
        testing = false
    }

    // MARK: 连接

    private func connectionSection(_ s: Status) -> some View {
        Section {
            if model.core.isLocal {
                if model.stopped {
                    Button {
                        Task { await model.restartLocal() }
                    } label: {
                        HStack {
                            Text("重新开始本机运行")
                            if model.restarting {
                                Spacer()
                                ProgressView()
                            }
                        }
                    }
                    .disabled(model.restarting)
                } else {
                    Button("停止本机运行", role: .destructive) { confirmStop = true }
                        .confirmationDialog("停止本机运行？", isPresented: $confirmStop, titleVisibility: .visible) {
                            Button("停止", role: .destructive) { Task { await model.stopLocal() } }
                        } message: {
                            Text("停止后定时任务不再运行，直到重新开始。账号和设置都保留。")
                        }
                }
            } else {
                LabeledContent("服务器", value: model.core.server)
            }
            Button("回到首页") { confirmBack = true }
                .confirmationDialog("回到首页？", isPresented: $confirmBack, titleVisibility: .visible) {
                    Button("回到首页", role: .destructive) { Task { await model.disconnect() } }
                } message: {
                    Text(model.core.isLocal
                        ? "会停止本机运行，账号和设置都保留，下次选「在手机上运行」时接着用。"
                        : "会退出这台服务器的面板登录并忘掉密码，服务器上的定时任务不受影响。")
                }
            LabeledContent("版本", value: s.version.isEmpty ? model.core.version : s.version)
        } header: {
            Text("连接")
        } footer: {
            Text(model.core.isLocal
                ? "定时任务在这台手机上运行。iOS 不允许 App 一直在后台运行：切到后台后定时任务暂停，回到 App 时补上。要不间断运行，请用服务器。"
                : "定时任务在服务器上运行。")
        }
    }

    // MARK: 保存

    @MainActor
    private func save() async {
        saving = true
        formError = nil
        do {
            let body = try JSONEncoder().encode(form.body)
            let s: PanelSettings = try await model.request("PUT", "/api/settings", body)
            let f = SettingsForm(s)
            form = f
            base = f
            model.notify("设置已保存")
            await model.refresh()
        } catch {
            if !model.isUnauthorized(error) { formError = error.localizedDescription }
        }
        saving = false
    }
}

/// 面板只收 jpg、png、gif、webp；照片 App 里常见的 HEIC 转成 JPEG
private func uploadData(_ data: Data) -> Data? {
    let b = [UInt8](data.prefix(12))
    let jpeg = b.count >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF
    let png = b.count >= 4 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47
    let gif = b.count >= 4 && b[0] == 0x47 && b[1] == 0x49 && b[2] == 0x46 && b[3] == 0x38
    let webp = b.count >= 12 && b[0] == 0x52 && b[1] == 0x49 && b[2] == 0x46 && b[3] == 0x46
        && b[8] == 0x57 && b[9] == 0x45 && b[10] == 0x42 && b[11] == 0x50
    if jpeg || png || gif || webp { return data }
    return UIImage(data: data)?.jpegData(compressionQuality: 0.9)
}

/// 一个心跳视频：封面、标题、UP 主
struct VideoRow: View {
    @EnvironmentObject var model: AppModel
    let bvid: String
    @State private var result: AppModel.VideoResult?

    var body: some View {
        let info: VideoInfo? = {
            if case .info(let i)? = result { return i }
            return nil
        }()
        HStack(spacing: 12) {
            LoadedImage(info.flatMap { hdslbURL($0.cover, size: "@192w_120h_1c.webp") }.map { ImageSource.bili($0) }) {
                Color.secondary.opacity(0.15)
            }
            .frame(width: 96, height: 60)
            .clipShape(RoundedRectangle(cornerRadius: 6))
            VStack(alignment: .leading, spacing: 2) {
                Text(info.map { $0.title.isEmpty ? bvid : $0.title } ?? bvid)
                    .font(.subheadline)
                    .lineLimit(2)
                switch result {
                case .info(let i)?:
                    Text([i.owner, Fmt.duration(i.duration), "\(Fmt.count(i.views)) 播放"].filter { !$0.isEmpty }.joined(separator: " · "))
                        .font(.caption)
                        .foregroundColor(.secondary)
                case .failed(let e)?:
                    Text(e).font(.caption).foregroundColor(.red)
                case nil:
                    Text("正在查询…").font(.caption).foregroundColor(.secondary)
                }
            }
            Spacer(minLength: 0)
            if let u = Links.video(bvid) {
                Link(destination: u) {
                    Image(systemName: "arrow.up.right.square")
                }
                .buttonStyle(.borderless)
                .accessibilityLabel("在 B 站打开")
            }
        }
        .task(id: bvid) { result = await model.video(bvid) }
    }
}
