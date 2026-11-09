import SwiftUI

/// 概览：目标属地、运行方式和每个账号的状态
struct OverviewView: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        NavigationStack {
            List {
                BannerSection()
                if let s = model.status {
                    zoneSection(s)
                    modeSection(s)
                    if s.accountList.isEmpty {
                        EmptyAccountsSection()
                    } else {
                        Section {
                            ForEach(s.accountList) { a in
                                Button {
                                    model.tab = .accounts
                                } label: {
                                    AccountSummaryRow(account: a)
                                }
                                .foregroundColor(.primary)
                            }
                        } header: {
                            Text(accountsHeader(s))
                        }
                        Section {
                            AsyncButton {
                                await model.runAll()
                            } label: {
                                Label("全部立即运行", systemImage: "play.fill")
                            }
                            .disabled(model.stopped)
                        }
                    }
                } else if model.statusError == nil {
                    LoadingRow()
                }
            }
            .refreshable { await model.refresh() }
            .navigationTitle("概览")
        }
    }

    private func zoneSection(_ s: Status) -> some View {
        Section {
            if let z = s.zone {
                VStack(alignment: .leading, spacing: 4) {
                    Text(z.region.isEmpty ? "未知" : z.region)
                        .font(.largeTitle.bold())
                        .foregroundColor(.brand)
                    Text([z.place, z.isp].filter { !$0.isEmpty }.joined(separator: " · "))
                    Text(z.addr + checkedText(s))
                        .font(.footnote)
                        .foregroundColor(.secondary)
                }
                .padding(.vertical, 4)
            } else {
                Text((s.zoneError ?? "").isEmpty ? "查询中" : "查不到")
                    .foregroundColor(.secondary)
            }
            if let e = s.zoneError, !e.isEmpty {
                Text(e).font(.footnote).foregroundColor(.red)
            }
        } header: {
            Text("目标属地 · \(whereLabel(s))")
        } footer: {
            Text("账号的主页属地会在定时请求之后几小时内变成这里，B 站没有公开具体规则。")
        }
    }

    private func checkedText(_ s: Status) -> String {
        guard let at = s.zoneCheckedAt, GoTime.isSet(at) else { return "" }
        return " · 查询于 \(Fmt.time(at))"
    }

    private func modeSection(_ s: Status) -> some View {
        Section {
            HStack {
                Text(model.core.isLocal ? "在手机上运行" : "连接服务器")
                Spacer()
                if model.core.isLocal && model.stopped {
                    Pill("已停止", color: .red)
                } else if model.core.isLocal {
                    Pill("运行中", color: .green)
                } else {
                    Pill("已连接", color: .green)
                }
            }
            if model.core.isLocal {
                let proxy = s.settings.proxy ?? ""
                LabeledContent("代理", value: proxy.isEmpty ? "没有，直接从手机发出" : proxy)
            } else {
                LabeledContent("服务器", value: model.core.server)
            }
            LabeledContent("版本", value: s.version.isEmpty ? model.core.version : s.version)
        } header: {
            Text("运行方式")
        } footer: {
            if model.core.isLocal {
                Text("iOS 不允许 App 一直在后台运行：切到后台后定时任务暂停，回到 App 时补上。")
            }
        }
    }

    private func accountsHeader(_ s: Status) -> String {
        let ok = s.accountList.filter { $0.loggedIn && $0.last?.ok == true }.count
        let bad = s.accountList.filter { !$0.loggedIn || $0.last?.ok == false }.count
        var parts = ["账号 \(s.accountList.count) 个"]
        if ok > 0 { parts.append("\(ok) 个正常") }
        if bad > 0 { parts.append("\(bad) 个需要处理") }
        return parts.joined(separator: " · ")
    }
}

/// 概览里的一行账号
struct AccountSummaryRow: View {
    let account: Account

    var body: some View {
        HStack(spacing: 12) {
            Avatar(account: account, size: 40)
            VStack(alignment: .leading, spacing: 2) {
                Text(account.label).lineLimit(1)
                Text(line)
                    .font(.footnote)
                    .foregroundColor(.secondary)
                    .lineLimit(1)
            }
            Spacer(minLength: 8)
            StateTag(account: account)
        }
    }

    private var line: String {
        let a = account
        var parts: [String] = []
        if let h = a.home, !h.text.isEmpty { parts.append(h.text) }
        if a.running {
            parts.append("正在运行")
        } else if let n = a.next {
            parts.append("下次 \(Fmt.time(n))")
        }
        if parts.isEmpty, let last = a.last { parts.append("上次 \(Fmt.time(last.at))") }
        return parts.isEmpty ? "UID \(a.mid)" : parts.joined(separator: " · ")
    }
}

/// 还没有账号
struct EmptyAccountsSection: View {
    @EnvironmentObject var model: AppModel

    var body: some View {
        Section {
            VStack(alignment: .leading, spacing: 8) {
                Text("还没有账号").font(.headline)
                Text("扫码登录一个 B 站账号，它就会按设置定时运行。可以添加多个，每个账号各自排程、同时进行。")
                    .font(.subheadline)
                    .foregroundColor(.secondary)
            }
            .padding(.vertical, 4)
            Button {
                model.qr = .add
            } label: {
                Label("扫码添加第一个账号", systemImage: "qrcode.viewfinder")
            }
            .disabled(model.stopped)
        }
    }
}
