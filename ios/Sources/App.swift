import Mobile
import UIKit
import WebKit

/// 记住的用法（remote 或 local）、服务器地址和代理
enum Prefs {
    private static let d = UserDefaults.standard
    static var mode: String { get { d.string(forKey: "mode") ?? "" } set { d.set(newValue, forKey: "mode") } }
    static var server: String { get { d.string(forKey: "server") ?? "" } set { d.set(newValue, forKey: "server") } }
    static var proxy: String { get { d.string(forKey: "proxy") ?? "" } set { d.set(newValue, forKey: "proxy") } }
    static var autoLocal: Bool { get { d.bool(forKey: "autoLocal") } set { d.set(newValue, forKey: "autoLocal") } }
}

/// 本机模式：Go 核心在 App 里运行（见 mobile/mobile.go）。iOS 不允许 App 在后台一直运行，
/// 切到后台后定时任务暂停，回到前台时按墙上时间补上。
enum Local {
    static func start(proxy: String) throws {
        let dir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("data", isDirectory: true)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        MobileSetTimeZone(TimeZone.current.identifier)
        var err: NSError?
        _ = MobileStart(dir.path, proxy, &err)
        if let err { throw err }
    }

    static func loginURL() throws -> URL {
        var err: NSError?
        let s = MobileLoginURL(&err)
        if let err { throw err }
        guard let u = URL(string: s) else { throw NSError(domain: "bia", code: 1, userInfo: [NSLocalizedDescriptionKey: "登录地址不对"]) }
        return u
    }
}

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?

    func application(_ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        let nav = UINavigationController(rootViewController: SetupViewController())
        window = UIWindow(frame: UIScreen.main.bounds)
        window?.rootViewController = nav
        window?.makeKeyAndVisible()
        return true
    }
}
