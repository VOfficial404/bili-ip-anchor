import SwiftUI

// 显示用的小工具，对应 webui/src/lib/format.ts 和 components/account-bits.tsx

extension Color {
    /// B 站粉 #FB7299
    static let brand = Color(hex: 0xFB7299)

    init(hex: UInt32) {
        self.init(
            red: Double((hex >> 16) & 0xff) / 255,
            green: Double((hex >> 8) & 0xff) / 255,
            blue: Double(hex & 0xff) / 255
        )
    }
}

enum Fmt {
    private static func two(_ n: Int) -> String { n < 10 ? "0\(n)" : "\(n)" }

    /// 今天 12:00、明天 08:30、11月3日 12:00
    static func time(_ d: Date) -> String {
        let cal = Calendar.current
        let c = cal.dateComponents([.month, .day, .hour, .minute], from: d)
        let hm = "\(two(c.hour ?? 0)):\(two(c.minute ?? 0))"
        let day = d.timeIntervalSince(cal.startOfDay(for: Date())) / 86400
        if day >= 0 && day < 1 { return "今天 \(hm)" }
        if day >= 1 && day < 2 { return "明天 \(hm)" }
        if day < 0 && day >= -1 { return "昨天 \(hm)" }
        return "\(c.month ?? 0)月\(c.day ?? 0)日 \(hm)"
    }

    /// 2026-11-03
    static func date(_ d: Date) -> String {
        let c = Calendar.current.dateComponents([.year, .month, .day], from: d)
        return "\(c.year ?? 0)-\(two(c.month ?? 0))-\(two(c.day ?? 0))"
    }

    /// 日志里的时间：11/03 12:00:05
    static func logTime(_ d: Date) -> String {
        let c = Calendar.current.dateComponents([.month, .day, .hour, .minute, .second], from: d)
        return "\(two(c.month ?? 0))/\(two(c.day ?? 0)) \(two(c.hour ?? 0)):\(two(c.minute ?? 0)):\(two(c.second ?? 0))"
    }

    /// 3 小时 20 分钟后、5 分钟前
    static func relative(_ d: Date) -> String {
        let sec = Int(d.timeIntervalSinceNow.rounded())
        let a = abs(sec)
        let text: String
        if a < 60 {
            text = "不到 1 分钟"
        } else if a < 3600 {
            text = "\(Int((Double(a) / 60).rounded())) 分钟"
        } else if a < 86400 * 2 {
            let h = a / 3600
            let m = Int((Double(a % 3600) / 60).rounded())
            text = m > 0 ? "\(h) 小时 \(m) 分钟" : "\(h) 小时"
        } else {
            text = "\(a / 86400) 天"
        }
        return sec >= 0 ? "\(text)后" : "\(text)前"
    }

    /// 视频时长：3:05、1:02:03
    static func duration(_ sec: Int) -> String {
        let m = sec / 60
        return m >= 60 ? "\(m / 60):\(two(m % 60)):\(two(sec % 60))" : "\(m):\(two(sec % 60))"
    }

    static func count(_ n: Int64) -> String {
        if n >= 100_000_000 { return String(format: "%.1f 亿", Double(n) / 1e8) }
        if n >= 10_000 { return String(format: "%.1f 万", Double(n) / 1e4) }
        return String(n)
    }

    /// 硬币可能有半个
    static func number(_ v: Double) -> String {
        v == v.rounded() ? String(Int64(v)) : String(format: "%.1f", v)
    }
}

/// 只接受 B 站图床的图片，加上缩略图参数
func hdslbURL(_ src: String, size: String) -> String? {
    var s = src.trimmingCharacters(in: .whitespaces)
    if s.hasPrefix("//") {
        s = "https:" + s
    } else if s.hasPrefix("http://") {
        s = "https://" + String(s.dropFirst(7))
    }
    guard s.hasPrefix("https://"), let u = URL(string: s), let host = u.host?.lowercased(),
          host.hasSuffix(".hdslb.com")
    else { return nil }
    return s + size
}

func initials(_ name: String) -> String {
    let t = name.trimmingCharacters(in: .whitespaces)
    let words = t.split(whereSeparator: { $0.isWhitespace })
    if words.count > 1, t.allSatisfy({ $0.isASCII }), let a = words[0].first, let b = words[1].first {
        return String([a, b]).uppercased()
    }
    return t.first.map { String($0) } ?? "?"
}

private let avatarColors: [UInt32] = [
    0xE11D48, 0xDB2777, 0xC026D3, 0x9333EA, 0x4F46E5, 0x0284C7,
    0x0891B2, 0x0D9488, 0x059669, 0x65A30D, 0xCA8A04, 0xEA580C,
]

func avatarColor(_ name: String) -> Color {
    var h: UInt32 = 0
    for ch in name.unicodeScalars {
        h = h &* 31 &+ ch.value
    }
    return Color(hex: avatarColors[Int(h % UInt32(avatarColors.count))])
}

/// 账号现在的状态和颜色
func accountState(_ a: Account) -> (String, Color) {
    if a.running { return ("运行中", .blue) }
    if let b = a.busy, !b.isEmpty { return ("正在\(b)", .blue) }
    if !a.loggedIn { return ("登录失效", .red) }
    guard let last = a.last else { return ("等待首次运行", .secondary) }
    return last.ok ? ("正常", .green) : ("上次失败", .orange)
}

/// 账号卡片上要提醒的错误
func accountError(_ a: Account) -> String? {
    if !a.loggedIn { return "这个账号的会话已失效，点「重新扫码」用同一个账号登录即可恢复。" }
    if let last = a.last, !last.ok, let e = last.error, !e.isEmpty { return e }
    return nil
}

/// 主页属地：显示的值、是否已是目标属地（nil 表示还不知道）、补充说明
struct HomeSummary {
    var value: String
    var pending: Bool
    var matched: Bool?
    var badge: String?
    var sub: String
}

func homeSummary(_ a: Account, zone: Zone?) -> HomeSummary {
    guard a.appAuth else {
        return HomeSummary(value: "未授权查询", pending: true, sub: "B 站只在 App 接口里提供主页属地，需要额外授权一次才能显示。")
    }
    guard let h = a.home, !(h.text.isEmpty && (h.error ?? "").isEmpty) else {
        return HomeSummary(value: "查询中", pending: true, sub: "")
    }
    if h.text.isEmpty {
        return HomeSummary(value: "查不到", pending: true, sub: h.error ?? "")
    }
    let target = zone?.region ?? ""
    var s = HomeSummary(value: h.text, pending: false, sub: "")
    if let since = h.matchedSince, GoTime.isSet(since) {
        s.matched = true
        s.badge = "已是目标属地"
        s.sub = "\(Fmt.time(since)) 起一致"
    } else if !target.isEmpty {
        s.matched = false
        s.badge = "还不是\(target)"
        let next = a.next.map { "下一轮（\(Fmt.time($0))）" } ?? "下一轮"
        s.sub = "预计\(next)之后几小时内变为\(target)，B 站没有公开具体规则"
    }
    if let e = h.error, !e.isEmpty {
        s.sub = "上次查询失败：\(e)"
    } else if let at = h.checkedAt, GoTime.isSet(at) {
        s.sub += (s.sub.isEmpty ? "" : "。") + "查询于 \(Fmt.time(at))"
    }
    return s
}

/// Cookie 的有效期
func cookieSummary(_ a: Account) -> (value: String, color: Color?, sub: String) {
    if let exp = a.cookie.expires, GoTime.isSet(exp) {
        let days = Int((exp.timeIntervalSinceNow / 86400).rounded(.up))
        let color: Color? = days <= 0 ? Color.red : (days < 14 ? Color.orange : nil)
        let sub = "到 \(Fmt.date(exp))" + (a.cookie.canRefresh ? "，B 站要求时会自动刷新" : "")
        return (days > 0 ? "还有 \(days) 天" : "已过期", color, sub)
    }
    return ("有效期未知", Color.secondary,
            a.cookie.canRefresh ? "下次运行时会查询" : "旧版本扫码登录的账号，重新扫码后可以看到有效期并刷新")
}

/// 运行：下次的时间和上次的结果
func runSummary(_ a: Account) -> (value: String, lines: [String]) {
    let value = a.running ? "正在运行" : a.next.map { "下次 \(Fmt.time($0))" } ?? "—"
    var lines: [String] = []
    if let n = a.next, !a.running { lines.append(Fmt.relative(n)) }
    if let last = a.last { lines.append("上次 \(Fmt.time(last.at)) \(last.ok ? "成功" : "失败")") }
    if let ok = a.lastOk, GoTime.isSet(ok) { lines.append("上次从目标 IP 发出请求：\(Fmt.time(ok))") }
    if lines.isEmpty { lines.append("还没有运行过") }
    return (value, lines)
}

/// B 站看到的是谁：本机或者服务器，经过代理时注明
func whereLabel(_ s: Status) -> String {
    let who = s.mode == "local" ? "本机" : "这台服务器"
    return (s.settings.proxy ?? "").isEmpty ? who : "\(who)（经代理）"
}

enum Links {
    static func video(_ bvid: String) -> URL? { URL(string: "https://www.bilibili.com/video/\(bvid)") }
    static func space(_ mid: String) -> URL? { URL(string: "https://space.bilibili.com/\(mid)") }
}
