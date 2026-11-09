import SwiftUI
import UIKit

// 几个页面共用的小部件

/// 头像、封面和动态图片的内存缓存
final class ImageCache {
    static let shared = ImageCache()
    private let cache = NSCache<NSString, UIImage>()

    init() { cache.countLimit = 200 }

    func get(_ key: String) -> UIImage? { cache.object(forKey: key as NSString) }
    func set(_ key: String, _ image: UIImage) { cache.setObject(image, forKey: key as NSString) }
}

/// 图片从哪里来
enum ImageSource: Equatable {
    case bili(String) // B 站图床的地址，用 MobileImage 下载
    case panel(String) // 面板接口的路径，例如动态图片

    var key: String {
        switch self {
        case .bili(let u): return u
        case .panel(let p): return "panel:" + p
        }
    }

    func fetch() async throws -> Data {
        switch self {
        case .bili(let u):
            return try await Core.image(u)
        case .panel(let p):
            let r = try await Core.call("GET", p)
            guard r.ok else { throw APIError(status: r.status, message: r.message) }
            return r.body
        }
    }
}

/// 加载并缩小图片，加载好之前和失败时显示 placeholder
struct LoadedImage<Placeholder: View>: View {
    let source: ImageSource?
    let placeholder: Placeholder
    @State private var image: UIImage? = nil

    init(_ source: ImageSource?, @ViewBuilder placeholder: () -> Placeholder) {
        self.source = source
        self.placeholder = placeholder()
    }

    var body: some View {
        ZStack {
            if let image {
                Image(uiImage: image).resizable().scaledToFill()
            } else {
                placeholder
            }
        }
        .task(id: source?.key) { await load() }
    }

    private func load() async {
        guard let source else {
            image = nil
            return
        }
        if let cached = ImageCache.shared.get(source.key) {
            image = cached
            return
        }
        image = nil
        guard let data = try? await source.fetch(), let full = UIImage(data: data) else { return }
        // 动态图片可能有 10 MB，列表里只要缩略图
        let side = max(full.size.width, full.size.height)
        let scale = side > 360 ? 360 / side : 1
        let size = CGSize(width: full.size.width * scale, height: full.size.height * scale)
        let thumb = await full.byPreparingThumbnail(ofSize: size) ?? full
        ImageCache.shared.set(source.key, thumb)
        image = thumb
    }
}

/// B 站头像，加载失败或没有时显示首字
struct Avatar: View {
    let account: Account
    @ScaledMetric private var size: CGFloat

    init(account: Account, size: CGFloat = 44) {
        self.account = account
        _size = ScaledMetric(wrappedValue: size, relativeTo: .body)
    }

    var body: some View {
        let src = hdslbURL(account.face, size: "@96w_96h_1c.webp")
        LoadedImage(src.map { ImageSource.bili($0) }) {
            ZStack {
                avatarColor(account.label)
                Text(initials(account.label))
                    .font(.system(size: size * 0.42, weight: .semibold))
                    .foregroundColor(.white)
            }
        }
        .frame(width: size, height: size)
        .clipShape(Circle())
        .accessibilityHidden(true)
    }
}

/// 小标签，例如「正常」「Lv6」
struct Pill: View {
    let text: String
    let color: Color

    init(_ text: String, color: Color = .secondary) {
        self.text = text
        self.color = color
    }

    var body: some View {
        Text(text)
            .font(.caption.weight(.medium))
            .foregroundColor(color)
            .lineLimit(1)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(color.opacity(0.14), in: Capsule())
    }
}

/// 账号状态的标签
struct StateTag: View {
    let account: Account

    var body: some View {
        let (text, color) = accountState(account)
        Pill(text, color: color)
    }
}

/// 点了之后运行 action，运行时显示转圈并且不能再点
struct AsyncButton<Label: View>: View {
    let role: ButtonRole?
    let action: () async -> Void
    let label: Label
    @State private var running = false

    init(role: ButtonRole? = nil, action: @escaping () async -> Void, @ViewBuilder label: () -> Label) {
        self.role = role
        self.action = action
        self.label = label()
    }

    var body: some View {
        Button(role: role) {
            running = true
            Task {
                await action()
                running = false
            }
        } label: {
            HStack(spacing: 8) {
                label
                if running {
                    Spacer(minLength: 0)
                    ProgressView()
                }
            }
        }
        .disabled(running)
    }
}

extension AsyncButton where Label == Text {
    init(_ title: String, role: ButtonRole? = nil, action: @escaping () async -> Void) {
        self.init(role: role, action: action) { Text(title) }
    }
}

/// 放在列表最上面：本机运行停止了、连不上服务等
struct BannerSection: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        if model.stopped {
            Section {
                Label {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("本机运行已经停止").font(.headline)
                        Text("账号和设置都还在，重新开始后继续定时运行。")
                            .font(.subheadline)
                            .foregroundColor(.secondary)
                    }
                } icon: {
                    Image(systemName: "pause.circle.fill").foregroundColor(.orange)
                }
                Button {
                    Task { await model.restartLocal() }
                } label: {
                    HStack {
                        Text("重新开始")
                        if model.restarting {
                            Spacer()
                            ProgressView()
                        }
                    }
                }
                .disabled(model.restarting)
            }
        } else if let err = model.statusError {
            Section {
                Label {
                    Text(model.status == nil ? err : "\(model.core.isRemote ? "连不上服务器" : "连不上服务")，显示的是之前的数据：\(err)")
                        .font(.subheadline)
                } icon: {
                    Image(systemName: "exclamationmark.triangle.fill").foregroundColor(.orange)
                }
                if model.status == nil {
                    AsyncButton("重试") { await model.refresh() }
                }
            }
        }
    }
}

/// 还没拿到状态时显示的转圈
struct LoadingRow: View {
    var body: some View {
        HStack {
            Spacer()
            ProgressView("正在读取")
            Spacer()
        }
        .padding(.vertical, 24)
        .listRowBackground(Color.clear)
    }
}

/// 屏幕下方的短提示
struct NoticeView: View {
    let text: String

    var body: some View {
        Text(text)
            .font(.subheadline.weight(.medium))
            .multilineTextAlignment(.center)
            .padding(.horizontal, 16)
            .padding(.vertical, 10)
            .background(.regularMaterial, in: Capsule())
            .shadow(color: .black.opacity(0.12), radius: 8, y: 2)
            .padding(.horizontal, 24)
            .transition(.move(edge: .bottom).combined(with: .opacity))
            .accessibilityAddTraits(.updatesFrequently)
    }
}
