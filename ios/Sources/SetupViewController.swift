import Mobile
import UIKit
import WebKit

/// 第一页：连接服务器，或者在本机运行。页面是 mobile/shell/setup.html，和 Android 共用；
/// 它通过 webkit.messageHandlers.bia 调用这里。这个桥只挂在这一页的 WebView 上，面板页没有。
class SetupViewController: UIViewController, WKScriptMessageHandlerWithReply, WKNavigationDelegate {
    private var web: WKWebView!
    private var autoStarted = false

    override func loadView() {
        let config = WKWebViewConfiguration()
        config.userContentController.addScriptMessageHandler(WeakHandler(self), contentWorld: .page, name: "bia")
        web = WKWebView(frame: .zero, configuration: config)
        web.navigationDelegate = self
        web.isOpaque = false
        web.backgroundColor = .systemGroupedBackground
        view = web
    }

    override func viewDidLoad() {
        super.viewDidLoad()
        if let url = Bundle.main.url(forResource: "setup", withExtension: "html") {
            web.loadFileURL(url, allowingReadAccessTo: url.deletingLastPathComponent())
        }
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        navigationController?.setNavigationBarHidden(true, animated: animated)
        web.evaluateJavaScript("window.biaRefresh && biaRefresh()")
    }

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        if autoStarted { return }
        autoStarted = true
        // 正在本机运行就直接打开面板；上次勾选了自动开始就开始运行
        if MobileRunning() {
            openLocal()
        } else if Prefs.mode == "local", Prefs.autoLocal, !Prefs.proxy.isEmpty {
            let proxy = Prefs.proxy
            DispatchQueue.global().async {
                if (try? Local.start(proxy: proxy)) != nil {
                    DispatchQueue.main.async { self.openLocal() }
                }
            }
        }
    }

    // 这一页不跳到别处
    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        decisionHandler(action.request.url?.isFileURL == true ? .allow : .cancel)
    }

    private func openLocal() {
        do {
            push(PanelViewController(url: try Local.loginURL(), local: true))
        } catch {
            web.evaluateJavaScript("window.biaRefresh && biaRefresh()")
        }
    }

    private func push(_ vc: UIViewController) {
        guard navigationController?.topViewController === self else { return }
        navigationController?.pushViewController(vc, animated: true)
    }

    func userContentController(_ uc: WKUserContentController, didReceive message: WKScriptMessage, replyHandler: @escaping (Any?, String?) -> Void) {
        guard message.webView === web, let body = message.body as? [String: Any], let cmd = body["cmd"] as? String else {
            replyHandler(nil, "不支持的调用")
            return
        }
        let args = body["args"] as? [String: Any] ?? [:]
        DispatchQueue.global().async {
            let out = self.handle(cmd, args)
            DispatchQueue.main.async { replyHandler(out, nil) }
        }
    }

    /// 在后台线程上执行，返回给页面的对象；出错时有 error
    private func handle(_ cmd: String, _ args: [String: Any]) -> [String: Any] {
        do {
            switch cmd {
            case "state":
                let running = MobileRunning()
                return ["mode": Prefs.mode, "server": Prefs.server, "proxy": running ? MobileProxy() : Prefs.proxy,
                        "autoLocal": Prefs.autoLocal, "running": running, "version": MobileVersion(), "platform": "ios"]
            case "probe":
                var err: NSError?
                let s = MobileProbeProxy(args["proxy"] as? String ?? "", &err)
                if let err { throw err }
                return (try JSONSerialization.jsonObject(with: Data(s.utf8)) as? [String: Any]) ?? [:]
            case "openRemote":
                let server = args["server"] as? String ?? ""
                guard let url = URL(string: server) else { return ["error": "地址不对"] }
                Prefs.mode = "remote"
                Prefs.server = server
                DispatchQueue.main.async { self.push(PanelViewController(url: url, local: false)) }
            case "startLocal":
                let proxy = (args["proxy"] as? String ?? "").trimmingCharacters(in: .whitespaces)
                try Local.start(proxy: proxy)
                Prefs.mode = "local"
                Prefs.proxy = proxy
                Prefs.autoLocal = args["auto"] as? Bool ?? false
                DispatchQueue.main.async { self.openLocal() }
            case "openLocal":
                DispatchQueue.main.async { self.openLocal() }
            case "stopLocal":
                MobileStop()
            default:
                return ["error": "未知命令：\(cmd)"]
            }
            return [:]
        } catch {
            return ["error": error.localizedDescription]
        }
    }
}

/// WKUserContentController 会强引用 handler，用它避免和页面互相引用
class WeakHandler: NSObject, WKScriptMessageHandlerWithReply {
    weak var target: WKScriptMessageHandlerWithReply?
    init(_ target: WKScriptMessageHandlerWithReply) { self.target = target }

    func userContentController(_ uc: WKUserContentController, didReceive message: WKScriptMessage, replyHandler: @escaping (Any?, String?) -> Void) {
        guard let target else { return replyHandler(nil, "页面已关闭") }
        target.userContentController(uc, didReceive: message, replyHandler: replyHandler)
    }
}
