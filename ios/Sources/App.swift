import Mobile
import UIKit
import WebKit

/// Go 核心（mobile/mobile.go）：在 127.0.0.1 上提供界面（和桌面客户端同一份 MyGo 代码编成的 WebAssembly）和接口。
/// iOS 不允许 App 在后台一直运行：切到后台后本机的定时任务暂停，回到前台时按墙上时间补上。
enum Core {
    /// 开始（已经开始过就不再开始），返回 WebView 要打开的一次性入口地址
    static func start() throws -> URL {
        guard let web = Bundle.main.url(forResource: "web", withExtension: nil) else {
            throw NSError(domain: "bia", code: 1, userInfo: [NSLocalizedDescriptionKey: "App 里没有界面文件"])
        }
        let data = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("data", isDirectory: true)
        try FileManager.default.createDirectory(at: data, withIntermediateDirectories: true)
        MobileSetTimeZone(TimeZone.current.identifier)
        var err: NSError?
        let s = MobileStart(web.path, data.path, &err)
        if let err { throw err }
        // 上次本机运行没有被用户停止（App 被系统关掉过）：接着运行
        _ = MobileResumeLocal(&err)
        if let err { throw err }
        guard let u = URL(string: s) else {
            throw NSError(domain: "bia", code: 2, userInfo: [NSLocalizedDescriptionKey: "入口地址不对"])
        }
        return u
    }

    /// 后台久了系统会收回监听的端口：关掉重新开始
    static func restart() throws -> URL {
        MobileShutdown()
        return try start()
    }
}

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?

    func application(_ application: UIApplication, didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        window = UIWindow(frame: UIScreen.main.bounds)
        window?.rootViewController = WebViewController()
        window?.makeKeyAndVisible()
        return true
    }
}

/// 唯一的页面：WKWebView 打开 Go 核心提供的界面。连接服务器、本机运行都在界面里选。
class WebViewController: UIViewController, WKNavigationDelegate, WKUIDelegate {
    private var web: WKWebView!
    private var origin: URL?

    override func loadView() {
        let config = WKWebViewConfiguration()
        // 界面用 window.open 打开 B 站视频等链接，在 createWebViewWith 里转到 Safari
        config.preferences.javaScriptCanOpenWindowsAutomatically = true
        web = WKWebView(frame: .zero, configuration: config)
        web.navigationDelegate = self
        web.uiDelegate = self
        web.scrollView.isScrollEnabled = false // 界面自己滚动
        web.scrollView.contentInsetAdjustmentBehavior = .never
        web.translatesAutoresizingMaskIntoConstraints = false
        // 界面不知道刘海和底部横条在哪，WebView 只占安全区域
        view = UIView()
        view.backgroundColor = .systemBackground
        view.addSubview(web)
        let safe = view.safeAreaLayoutGuide
        NSLayoutConstraint.activate([
            web.topAnchor.constraint(equalTo: safe.topAnchor),
            web.bottomAnchor.constraint(equalTo: safe.bottomAnchor),
            web.leadingAnchor.constraint(equalTo: safe.leadingAnchor),
            web.trailingAnchor.constraint(equalTo: safe.trailingAnchor),
        ])
    }

    override func viewDidLoad() {
        super.viewDidLoad()
        load(Core.start)
        NotificationCenter.default.addObserver(self, selector: #selector(foreground), name: UIApplication.willEnterForegroundNotification, object: nil)
    }

    private func load(_ entry: () throws -> URL) {
        do {
            let u = try entry()
            origin = u
            web.load(URLRequest(url: u))
        } catch {
            let alert = UIAlertController(title: "打不开", message: error.localizedDescription, preferredStyle: .alert)
            present(alert, animated: true)
        }
    }

    /// 回到前台时看看界面的服务还在不在，不在就重新开始并重新打开界面
    @objc private func foreground() {
        guard let origin, let ping = URL(string: "/host/state", relativeTo: origin) else { return }
        var req = URLRequest(url: ping, timeoutInterval: 2)
        req.httpShouldHandleCookies = false
        URLSession.shared.dataTask(with: req) { _, resp, _ in
            if resp != nil { return } // 有回应（没有 Cookie 时是 403）就是还在
            DispatchQueue.main.async { self.load(Core.restart) }
        }.resume()
    }

    // WebView 的进程被系统收回：用新的入口地址重新打开界面
    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        load(Core.start)
    }

    private func sameOrigin(_ u: URL) -> Bool {
        guard let origin else { return false }
        return u.scheme == origin.scheme && u.host == origin.host && u.port == origin.port
    }

    // 界面以外的链接用 Safari 打开
    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let u = action.request.url else { return decisionHandler(.cancel) }
        if sameOrigin(u) || u.scheme == "about" || u.scheme == "blob" {
            return decisionHandler(.allow)
        }
        UIApplication.shared.open(u)
        decisionHandler(.cancel)
    }

    // window.open 和 target="_blank"
    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for action: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        if let u = action.request.url, !sameOrigin(u) { UIApplication.shared.open(u) }
        return nil
    }
}
