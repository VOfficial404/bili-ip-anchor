import SwiftUI
import UIKit

/// 扫码。target.mid 为 nil 时是网页扫码登录（添加账号或重新登录），否则是 TV 扫码授权，用来查询这个账号的主页属地。
struct QRView: View {
    @EnvironmentObject var model: AppModel
    @Environment(\.dismiss) private var dismiss
    let target: QRTarget

    enum Phase { case loading, waiting, scanned, expired, failed }

    @State private var round = 0
    @State private var phase: Phase = .loading
    @State private var image: UIImage?
    @State private var error: String?

    private var again: Bool { phase == .expired || phase == .failed }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 16) {
                    ZStack {
                        // 二维码在深色模式下也要白底
                        Color.white
                        if let image, phase != .loading, phase != .failed {
                            Image(uiImage: image)
                                .interpolation(.none)
                                .resizable()
                                .scaledToFit()
                                .padding(8)
                                .opacity(again || phase == .scanned ? 0.2 : 1)
                        } else if phase == .loading {
                            ProgressView()
                        }
                        if again {
                            Button("刷新二维码") { round += 1 }
                                .buttonStyle(.borderedProminent)
                        } else if phase == .scanned {
                            Image(systemName: "checkmark.circle.fill")
                                .font(.system(size: 56))
                                .foregroundColor(.green)
                        }
                    }
                    .frame(width: 240, height: 240)
                    .clipShape(RoundedRectangle(cornerRadius: 16))
                    .overlay(RoundedRectangle(cornerRadius: 16).stroke(Color.secondary.opacity(0.2)))

                    Text(stateText)
                        .font(.headline)
                        .foregroundColor(again ? .red : .primary)
                    if let error {
                        Text(error)
                            .font(.footnote)
                            .foregroundColor(.red)
                            .multilineTextAlignment(.center)
                    }

                    Text(hint)
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                        .multilineTextAlignment(.center)

                    if let image, phase == .waiting {
                        let picture = Image(uiImage: image)
                        ShareLink(item: picture, preview: SharePreview("登录二维码", image: picture)) {
                            Label("存储或分享二维码", systemImage: "square.and.arrow.up")
                        }
                        .buttonStyle(.bordered)
                        Text("二维码就在这台手机上时，可以存到相册，再在 B 站 App 的「扫一扫」里从相册选这张图；也可以用另一台手机扫。")
                            .font(.caption)
                            .foregroundColor(.secondary)
                            .multilineTextAlignment(.center)
                    }
                }
                .padding(24)
                .frame(maxWidth: .infinity)
            }
            .navigationTitle(target.mid == nil ? "扫码登录" : "授权查询主页属地")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(again ? "关闭" : "取消") { dismiss() }
                }
            }
        }
        .task(id: round) { await run() }
    }

    private var stateText: String {
        switch phase {
        case .loading: return "正在获取二维码…"
        case .waiting: return "等待扫码"
        case .scanned: return "已扫码，请在 App 里确认"
        case .expired: return "二维码已过期"
        case .failed: return "获取二维码失败"
        }
    }

    private var hint: String {
        if target.mid == nil {
            return "用 B 站 App 的「扫一扫」扫码并确认登录。"
        }
        return "用「\(target.label)」这个账号的 B 站 App 扫码并确认。只用来查询主页显示的 IP 属地，可以随时取消。"
    }

    @MainActor
    private func run() async {
        phase = .loading
        error = nil
        do {
            let r = try await model.send("POST", target.path)
            guard let img = UIImage(data: r.body) else { throw BridgeError("二维码不是图片") }
            image = img
            phase = .waiting
        } catch {
            self.error = error.localizedDescription
            phase = .failed
            return
        }
        while !Task.isCancelled {
            try? await Task.sleep(nanoseconds: 2_000_000_000)
            if Task.isCancelled { return }
            do {
                let p: QRPoll = try await model.request("GET", target.path)
                error = nil
                switch p.state {
                case "scanned":
                    phase = .scanned
                case "expired", "none":
                    phase = .expired
                    return
                case "success":
                    model.qrSucceeded(target, mid: p.mid)
                    dismiss()
                    return
                default:
                    phase = .waiting
                }
            } catch {
                // 服务器要求重新输入密码时会弹出密码框；其他错误先显示，接着查
                if model.isUnauthorized(error) { return }
                self.error = error.localizedDescription
            }
        }
    }
}
