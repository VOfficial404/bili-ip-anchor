import Foundation
import Mobile

/// Go 核心（mobile/mobile.go）的 Swift 封装。
/// Go 的函数都可以在后台线程上同时调用；会访问网络的不能在主线程上调用，所以这里全部放进 Task.detached。
enum Core {
    /// 在后台线程上运行（可能出错的）Go 调用
    static func run<T: Sendable>(_ work: @escaping @Sendable () throws -> T) async throws -> T {
        try await Task.detached(priority: .userInitiated) { try work() }.value
    }

    /// 在后台线程上运行不会出错的 Go 调用
    static func background<T: Sendable>(_ work: @escaping @Sendable () -> T) async -> T {
        await Task.detached(priority: .userInitiated) { work() }.value
    }

    static var version: String {
        let v: String? = MobileVersion()
        return v ?? ""
    }

    /// 打开数据目录（Application Support/data，和以前一样），接着运行上次没有被用户停止的本机运行
    static func open() throws -> Opened {
        MobileSetTimeZone(TimeZone.current.identifier)
        let dir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("data", isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        var err: NSError?
        _ = MobileOpen(dir.path, &err)
        if let err { throw err }
        // 上次本机运行没有被用户停止（App 被系统关掉过）：接着运行
        var resumeErr: NSError?
        _ = MobileResumeLocal(&resumeErr)
        return Opened(state: state(), resumeError: resumeErr?.localizedDescription)
    }

    /// 回到前台时调用：本机运行被系统停下过就接着运行；已经在运行时什么也不做。
    /// 在运行时不能调用 ResumeLocal：它会用 mobile-local 里记住的代理覆盖设置里的代理。
    static func resumeIfStopped() -> String? {
        if MobileLocalRunning() { return nil }
        var err: NSError?
        _ = MobileResumeLocal(&err)
        return err?.localizedDescription
    }

    static func state() -> CoreState {
        let s: String? = MobileState()
        guard let data = s?.data(using: .utf8),
              let st = try? JSONDecoder.go().decode(CoreState.self, from: data)
        else { return CoreState() }
        return st
    }

    static func probeProxy(_ proxy: String) throws -> Zone {
        var err: NSError?
        let s: String? = MobileProbeProxy(proxy, &err)
        if let err { throw err }
        return try decode(Zone.self, from: Data((s ?? "").utf8))
    }

    static func startLocal(_ proxy: String) throws -> LocalStart {
        var err: NSError?
        let s: String? = MobileStartLocal(proxy, &err)
        if let err { throw err }
        return try decode(LocalStart.self, from: Data((s ?? "").utf8))
    }

    static func stopLocal() { MobileStopLocal() }

    static func connectServer(_ server: String, password: String, remember: Bool) throws {
        var err: NSError?
        _ = MobileConnectServer(server, password, remember, &err)
        if let err { throw err }
    }

    static func disconnect() { MobileDisconnect() }

    /// 发一个面板接口请求。只有连不上时才抛出错误；接口的错误在 APIResponse 里
    static func call(_ method: String, _ path: String, _ body: Data? = nil) async throws -> APIResponse {
        try await run {
            var err: NSError?
            let r = MobileCall(method, path, body, &err)
            if let err { throw err }
            guard let r else { throw BridgeError("没有收到回应") }
            let data: Data? = r.body
            let filename: String? = r.filename
            let message: String? = r.message()
            return APIResponse(status: Int(r.status), body: data ?? Data(), filename: filename ?? "", message: message ?? "")
        }
    }

    /// 下载 B 站图床的图片
    static func image(_ url: String) async throws -> Data {
        try await run {
            var err: NSError?
            let d: Data? = MobileImage(url, &err)
            if let err { throw err }
            guard let d else { throw BridgeError("没有图片") }
            return d
        }
    }

    static func decode<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        do {
            return try JSONDecoder.go().decode(type, from: data)
        } catch {
            throw BridgeError("看不懂返回的数据：\(error.localizedDescription)")
        }
    }
}

struct Opened: Sendable {
    let state: CoreState
    let resumeError: String?
}

struct BridgeError: LocalizedError, Sendable {
    let message: String
    init(_ message: String) { self.message = message }
    var errorDescription: String? { message }
}

/// Call 的结果
struct APIResponse: Sendable {
    let status: Int
    let body: Data
    let filename: String
    let message: String
    var ok: Bool { (200..<300).contains(status) }
}

/// 接口返回的错误（状态码不是 2xx）
struct APIError: LocalizedError, Sendable {
    let status: Int
    let message: String
    var errorDescription: String? { message }
}

extension JSONDecoder {
    /// Go 的 JSON：键是 snake_case，时间是 RFC 3339（可能带纳秒）
    static func go() -> JSONDecoder {
        let d = JSONDecoder()
        d.keyDecodingStrategy = .convertFromSnakeCase
        d.dateDecodingStrategy = .custom { decoder in
            let c = try decoder.singleValueContainer()
            let s = try c.decode(String.self)
            return GoTime.parse(s) ?? .distantPast
        }
        return d
    }
}

enum GoTime {
    /// 解析 Go 的 time.Time。ISO8601DateFormatter 对小数秒的位数有限制，所以先拿掉小数部分再加回去
    static func parse(_ s: String) -> Date? {
        var str = s
        var fraction = 0.0
        if let t = str.firstIndex(of: "T"), let dot = str[t...].firstIndex(of: ".") {
            var end = str.index(after: dot)
            while end < str.endIndex, str[end].isASCII, str[end].isNumber {
                end = str.index(after: end)
            }
            fraction = Double("0" + String(str[dot..<end])) ?? 0
            str.removeSubrange(dot..<end)
        }
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        guard let d = f.date(from: str) else { return nil }
        return d.addingTimeInterval(fraction)
    }

    /// Go 的零值时间（0001-01-01）表示没有
    static func isSet(_ d: Date?) -> Bool {
        guard let d else { return false }
        return d.timeIntervalSince1970 > 0
    }
}

// MARK: - 数据（对应 webui/src/lib/api.ts 和 mobile.State）

/// mobile.State() 的内容
struct CoreState: Decodable, Sendable, Equatable {
    var mode = ""
    var server = ""
    var proxy = ""
    var remember = false
    var localRunning = false
    var version = ""

    static let local = "local"
    static let remote = "remote"

    var isLocal: Bool { mode == CoreState.local }
    var isRemote: Bool { mode == CoreState.remote }

    init() {}

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? ""
        server = try c.decodeIfPresent(String.self, forKey: .server) ?? ""
        proxy = try c.decodeIfPresent(String.self, forKey: .proxy) ?? ""
        remember = try c.decodeIfPresent(Bool.self, forKey: .remember) ?? false
        localRunning = try c.decodeIfPresent(Bool.self, forKey: .localRunning) ?? false
        version = try c.decodeIfPresent(String.self, forKey: .version) ?? ""
    }

    private enum CodingKeys: String, CodingKey {
        case mode, server, proxy, remember, localRunning, version
    }
}

struct Zone: Codable, Sendable, Equatable {
    var addr: String
    var country: String
    var province: String
    var city: String
    var isp: String

    /// 显示用的地点：国内是省份和城市，国外是国家和地区
    var place: String {
        let parts = country == "中国" ? [province, city] : [country, province]
        var seen: [String] = []
        for p in parts where !p.isEmpty && !seen.contains(p) { seen.append(p) }
        return seen.isEmpty ? "未知地区" : seen.joined(separator: " ")
    }

    /// 主页属地显示的那一级：国内是省份，国外是国家
    var region: String {
        country == "中国" && !province.isEmpty ? province : country
    }
}

/// StartLocal 的结果
struct LocalStart: Decodable, Sendable {
    var zone: Zone?
    var warning: String?
}

struct PanelSettings: Decodable, Sendable, Equatable {
    var videos: [String]?
    var doDynamic: Bool
    var dynamicText: String?
    var interval: String
    var jitter: String
    var proxy: String?
    var logLevel: String?
}

/// PUT /api/settings 的请求体，和网页面板一样
struct SettingsBody: Encodable {
    var videos: [String]
    var doDynamic: Bool
    var dynamicText: String
    var interval: String
    var jitter: String
    var proxy: String
    var logLevel: String

    enum CodingKeys: String, CodingKey {
        case videos
        case doDynamic = "do_dynamic"
        case dynamicText = "dynamic_text"
        case interval, jitter, proxy
        case logLevel = "log_level"
    }
}

struct RunResult: Decodable, Sendable, Equatable {
    var at: Date
    var ok: Bool
    var error: String?
}

struct Stats: Decodable, Sendable, Equatable {
    var coins: Double
    var bcoins: Double
    var level: Int
    var vip: String?
    var updatedAt: Date?
}

struct CookieStatus: Decodable, Sendable, Equatable {
    var expires: Date?
    var canRefresh: Bool
}

struct HomeLocation: Decodable, Sendable, Equatable {
    var text: String
    var checkedAt: Date?
    var matchedSince: Date?
    var error: String?
}

/// 导入收藏的进度，见 core/favorites.go
struct FavImport: Decodable, Sendable, Equatable {
    var running: Bool
    var folder: String?
    var total: Int
    var done: Int
    var added: Int
    var skipped: Int
    var failed: Int
    var failures: [FavFailure]?
    var created: [String]?
    var error: String?
    var started: Date?
    var finished: Date?
}

struct FavFailure: Decodable, Sendable, Equatable {
    var bvid: String
    var title: String?
    var reason: String
}

struct Account: Decodable, Sendable, Equatable, Identifiable {
    var mid: String
    var uname: String
    var face: String
    var loggedIn: Bool
    var running: Bool
    var busy: String?
    var last: RunResult?
    var lastOk: Date?
    var next: Date?
    var stats: Stats?
    var cookie: CookieStatus
    var appAuth: Bool
    var home: HomeLocation?
    var favImport: FavImport?

    var id: String { mid }
    var label: String { uname.isEmpty ? "UID \(mid)" : uname }
    var isBusy: Bool { running || !(busy ?? "").isEmpty }
}

struct ImageInfo: Decodable, Sendable, Equatable, Identifiable {
    var name: String
    var size: Int
    var id: String { name }
}

struct Status: Decodable, Sendable, Equatable {
    var version: String
    var mode: String
    var accounts: [Account]?
    var zone: Zone?
    var zoneError: String?
    var zoneCheckedAt: Date?
    var settings: PanelSettings
    var images: [ImageInfo]?
    var logLevel: String?

    var accountList: [Account] { accounts ?? [] }
    var imageList: [ImageInfo] { images ?? [] }
}

struct LogEntry: Decodable, Sendable, Equatable, Identifiable {
    var seq: Int64
    var time: Date
    var level: String
    var mid: String?
    var account: String?
    var msg: String
    var id: Int64 { seq }
}

struct LogsResponse: Decodable, Sendable {
    var level: String?
    var entries: [LogEntry]?
}

struct VideoInfo: Decodable, Sendable, Equatable {
    var bvid: String
    var title: String
    var cover: String
    var owner: String
    var duration: Int
    var views: Int64
}

struct QRPoll: Decodable, Sendable {
    var state: String
    var mid: String?
}

struct RunAllResult: Decodable, Sendable {
    var triggered: Int
}

struct FavImportStarted: Decodable, Sendable {
    var total: Int
}

/// 导出的收藏夹文件，导入前读出来让用户确认
struct FavFile: Decodable, Sendable {
    struct Folder: Decodable, Sendable {
        var title: String?
        var `default`: Bool?
        var videos: [Video]?
    }

    struct Video: Decodable, Sendable {
        var invalid: Bool?
    }

    var format: String?
    var version: Int?
    var mid: String?
    var uname: String?
    var folders: [Folder]?

    static let formatName = "bili-ip-anchor/favorites"
}
