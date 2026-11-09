import SwiftUI

/// 扫码：添加账号（mid 为 nil），或者授权查询某个账号的主页属地
struct QRTarget: Identifiable, Equatable {
    let mid: String?
    let label: String

    static let add = QRTarget(mid: nil, label: "")

    var id: String { mid ?? "" }
    var path: String { mid.map { "/api/accounts/\($0)/appauth" } ?? "/api/qr" }
}

struct AlertMessage: Identifiable {
    let id = UUID()
    let title: String
    let message: String
}

enum MainTab: Hashable {
    case overview, accounts, settings, logs
}

/// 界面的全部状态。Go 的调用都在后台线程上进行（见 Go），结果在主线程上发布。
@MainActor
final class AppModel: ObservableObject {
    enum Phase: Equatable {
        case loading
        case failed(String)
        case setup
        case main
    }

    enum VideoResult: Equatable {
        case info(VideoInfo)
        case failed(String)
    }

    @Published var phase: Phase = .loading
    @Published var core = CoreState()
    @Published var status: Status?
    @Published var statusError: String?
    @Published var needsPassword = false
    @Published var qr: QRTarget?
    @Published var tab: MainTab = .overview
    @Published var alert: AlertMessage?
    @Published var notice: String?
    @Published var restarting = false

    private var noticeTask: Task<Void, Never>?
    private var videoCache: [String: VideoResult] = [:]

    /// 本机运行被停下了：接口都返回 409
    var stopped: Bool { core.isLocal && !core.localRunning }

    // MARK: 启动和用法

    func start() async {
        guard phase == .loading else { return }
        do {
            let opened = try await Core.run { try Core.open() }
            core = opened.state
            if let e = opened.resumeError {
                alert = AlertMessage(title: "没能接着在手机上运行", message: e)
            }
            phase = core.mode.isEmpty ? .setup : .main
        } catch {
            phase = .failed(error.localizedDescription)
        }
    }

    func retryStart() {
        phase = .loading
        Task { await start() }
    }

    /// 回到前台。iOS 不允许 App 在后台一直运行：切到后台后本机的定时任务暂停，回到前台时按墙上时间补上。
    /// 没有监听的端口（Call 直接交给定时任务），所以不用像以前那样重新开始；
    /// 只有定时任务不在运行、又不是用户停止的时候才接着运行。
    /// 在运行时不能调用 ResumeLocal：它会用 mobile-local 里记住的代理覆盖设置里的代理。
    func becameActive() async {
        guard phase == .main else { return }
        await refreshCore()
        if core.isLocal && !core.localRunning {
            let err = await Core.background { Core.resumeIfStopped() }
            if let err { alert = AlertMessage(title: "没能接着在手机上运行", message: err) }
        }
        await refresh()
    }

    func refreshCore() async {
        core = await Core.background { Core.state() }
    }

    /// 选好用法之后进入主界面
    func enterMain() {
        status = nil
        statusError = nil
        needsPassword = false
        videoCache = [:]
        tab = .overview
        phase = .main
        Task { await refresh() }
    }

    func startLocal(_ proxy: String) async throws -> LocalStart {
        let p = proxy.trimmingCharacters(in: .whitespacesAndNewlines)
        let r = try await Core.run { try Core.startLocal(p) }
        await refreshCore()
        return r
    }

    func connect(server: String, password: String, remember: Bool) async throws {
        let s = server.trimmingCharacters(in: .whitespacesAndNewlines)
        try await Core.run { try Core.connectServer(s, password: password, remember: remember) }
        await refreshCore()
    }

    /// 本机运行停止后用记住的代理重新开始
    func restartLocal() async {
        restarting = true
        defer { restarting = false }
        let proxy = core.proxy
        do {
            let r = try await Core.run { try Core.startLocal(proxy) }
            if let w = r.warning, !w.isEmpty {
                alert = AlertMessage(title: "已重新开始", message: w)
            } else {
                notify("已重新开始在手机上运行")
            }
        } catch {
            report(error, title: "没能重新开始")
        }
        await refreshCore()
        await refresh()
    }

    func stopLocal() async {
        await Core.background { Core.stopLocal() }
        await refreshCore()
        notify("已停止本机运行，账号和设置都还在")
    }

    /// 回到首页：停下本机运行，或者退出服务器的登录并忘掉密码。账号和设置都保留。
    func disconnect() async {
        await Core.background { Core.disconnect() }
        core = await Core.background { Core.state() }
        status = nil
        statusError = nil
        needsPassword = false
        qr = nil
        videoCache = [:]
        phase = .setup
    }

    // MARK: 面板接口

    func refresh() async {
        guard phase == .main else { return }
        await refreshCore()
        if needsPassword { return }
        do {
            let s: Status = try await request("GET", "/api/status")
            status = s
            statusError = nil
        } catch {
            statusError = error.localizedDescription
        }
    }

    /// 发请求；状态码不是 2xx 时抛出 APIError。401 打开密码输入，409 时重新看一下本机运行是不是停了
    @discardableResult
    func send(_ method: String, _ path: String, _ body: Data? = nil) async throws -> APIResponse {
        let r = try await Core.call(method, path, body)
        if !r.ok {
            if r.status == 401 && core.isRemote {
                needsPassword = true
            }
            if r.status == 409 && core.isLocal {
                await refreshCore()
            }
            let msg = r.message.isEmpty ? "请求失败（HTTP \(r.status)）" : r.message
            throw APIError(status: r.status, message: msg)
        }
        return r
    }

    func request<T: Decodable>(_ method: String, _ path: String, _ body: Data? = nil) async throws -> T {
        let r = try await send(method, path, body)
        return try Core.decode(T.self, from: r.body)
    }

    func isUnauthorized(_ error: Error) -> Bool {
        (error as? APIError)?.status == 401
    }

    /// 把错误显示成提示框；要重新输入服务器密码时不提示（会弹出密码输入）
    func report(_ error: Error, title: String = "出错了") {
        if isUnauthorized(error) && core.isRemote { return }
        alert = AlertMessage(title: title, message: error.localizedDescription)
    }

    /// 屏幕下方的短提示，几秒后消失
    func notify(_ text: String) {
        noticeTask?.cancel()
        withAnimation { notice = text }
        noticeTask = Task {
            try? await Task.sleep(nanoseconds: 2_500_000_000)
            if Task.isCancelled { return }
            withAnimation { notice = nil }
        }
    }

    func runAll() async {
        do {
            let r: RunAllResult = try await request("POST", "/api/run")
            notify("已让 \(r.triggered) 个账号立即运行")
        } catch {
            report(error, title: "没有运行")
        }
        try? await Task.sleep(nanoseconds: 600_000_000)
        await refresh()
    }

    /// 账号的操作，例如 act(mid, "POST", "run")；action 为空时是账号本身（移除）
    @discardableResult
    func act(_ mid: String, _ method: String, _ action: String, done: String?, failTitle: String) async -> Bool {
        let path = "/api/accounts/\(mid)" + (action.isEmpty ? "" : "/" + action)
        var ok = false
        do {
            try await send(method, path)
            if let done { notify(done) }
            ok = true
        } catch {
            report(error, title: failTitle)
        }
        await refresh()
        return ok
    }

    func qrSucceeded(_ target: QRTarget, mid: String?) {
        if target.mid != nil {
            notify("已授权，正在查询「\(target.label)」的主页属地")
        } else {
            notify("已添加账号 UID \(mid ?? "")，正在运行第一轮")
            tab = .accounts
        }
        Task {
            await refresh()
            try? await Task.sleep(nanoseconds: 3_000_000_000)
            await refresh()
        }
    }

    // MARK: 心跳视频

    /// 视频的标题和封面，查过的记住
    func video(_ bvid: String) async -> VideoResult? {
        if let v = videoCache[bvid] { return v }
        do {
            return .info(try await lookupVideo(bvid))
        } catch {
            if isUnauthorized(error) { return nil }
            let r = VideoResult.failed(error.localizedDescription)
            videoCache[bvid] = r
            return r
        }
    }

    func lookupVideo(_ bvid: String) async throws -> VideoInfo {
        let info: VideoInfo = try await request("GET", "/api/videos/\(bvid)")
        videoCache[bvid] = .info(info)
        return info
    }
}
