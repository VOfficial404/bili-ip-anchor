import Mobile
import UIKit
import WebKit

/// 面板：在 WebView 里打开 WebUI。本机模式打开自己开在 127.0.0.1 上的，否则打开服务器的。
class PanelViewController: UIViewController, WKNavigationDelegate, WKUIDelegate {
    private let url: URL
    private let local: Bool
    private var web: WKWebView!

    init(url: URL, local: Bool) {
        self.url = url
        self.local = local
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) { fatalError("不从 storyboard 创建") }

    override func loadView() {
        web = WKWebView(frame: .zero, configuration: WKWebViewConfiguration())
        web.navigationDelegate = self
        web.uiDelegate = self
        web.allowsBackForwardNavigationGestures = true
        view = web
    }

    override func viewDidLoad() {
        super.viewDidLoad()
        title = local ? "本机运行" : (url.host ?? "服务器")
        navigationItem.leftBarButtonItem = UIBarButtonItem(title: "首页", style: .plain, target: self, action: #selector(back))
        if local {
            navigationItem.rightBarButtonItem = UIBarButtonItem(title: "停止运行", style: .plain, target: self, action: #selector(stop))
            navigationItem.rightBarButtonItem?.tintColor = .systemRed
        }
        web.load(URLRequest(url: url))
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        navigationController?.setNavigationBarHidden(false, animated: animated)
    }

    @objc private func back() { navigationController?.popViewController(animated: true) }

    @objc private func stop() {
        let alert = UIAlertController(title: "停止本机运行？", message: "账号和设置都还在，下次开始运行时接着用。", preferredStyle: .alert)
        alert.addAction(UIAlertAction(title: "取消", style: .cancel))
        alert.addAction(UIAlertAction(title: "停止", style: .destructive) { _ in
            MobileStop()
            self.back()
        })
        present(alert, animated: true)
    }

    private func sameOrigin(_ u: URL) -> Bool {
        u.scheme == url.scheme && u.host == url.host && u.port == url.port
    }

    // 面板以外的链接（B 站视频、GitHub）用浏览器打开
    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let u = action.request.url else { return decisionHandler(.cancel) }
        if sameOrigin(u) || u.scheme == "about" || u.scheme == "blob" {
            return decisionHandler(.allow)
        }
        UIApplication.shared.open(u)
        decisionHandler(.cancel)
    }

    // target="_blank" 的链接
    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for action: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        if let u = action.request.url { UIApplication.shared.open(u) }
        return nil
    }
}
