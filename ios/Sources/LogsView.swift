import SwiftUI

private let levelRank = ["debug": 0, "info": 1, "warn": 2, "error": 3]

private func rank(_ level: String) -> Int { levelRank[level.lowercased()] ?? 1 }

private let maxLogs = 1000 // 和服务端日志缓冲一样多

/// 日志：最新的在最下面。第一次拉全部，之后只拉序号更大的。
struct LogsView: View {
    @EnvironmentObject var model: AppModel
    @State private var entries: [LogEntry] = []
    @State private var lastSeq: Int64 = 0
    @State private var error: String?
    @State private var minLevel = "debug"

    private var shown: [LogEntry] { entries.filter { rank($0.level) >= rank(minLevel) } }

    var body: some View {
        NavigationStack {
            ScrollViewReader { proxy in
                List {
                    if let error {
                        Text(error).font(.footnote).foregroundColor(.red)
                    }
                    if shown.isEmpty {
                        Text(entries.isEmpty ? "还没有日志" : "这个级别没有日志")
                            .foregroundColor(.secondary)
                    }
                    ForEach(shown) { e in
                        LogRow(entry: e).id(e.seq)
                    }
                }
                .listStyle(.plain)
                .onChange(of: shown.last?.seq) { seq in
                    if let seq { proxy.scrollTo(seq, anchor: .bottom) }
                }
            }
            .navigationTitle("日志")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .primaryAction) {
                    Menu {
                        Picker("级别", selection: $minLevel) {
                            Text("全部").tag("debug")
                            Text("信息及以上").tag("info")
                            Text("警告及以上").tag("warn")
                            Text("只看错误").tag("error")
                        }
                    } label: {
                        Label("筛选", systemImage: "line.3.horizontal.decrease.circle")
                    }
                }
            }
        }
        .task {
            while !Task.isCancelled {
                await poll()
                try? await Task.sleep(nanoseconds: 3_000_000_000)
            }
        }
        // 重新开始本机运行后日志序号从头开始
        .onChange(of: model.core.localRunning) { running in
            if running {
                entries = []
                lastSeq = 0
            }
        }
    }

    @MainActor
    private func poll() async {
        guard model.phase == .main, !model.needsPassword else { return }
        do {
            let r: LogsResponse = try await model.request("GET", "/api/logs?after=\(lastSeq)")
            error = nil
            let new = (r.entries ?? []).filter { $0.seq > lastSeq }
            if let last = new.last {
                lastSeq = last.seq
                entries = Array((entries + new).suffix(maxLogs))
            }
        } catch {
            if model.isUnauthorized(error) || model.stopped { return }
            self.error = error.localizedDescription
        }
    }
}

struct LogRow: View {
    let entry: LogEntry

    var body: some View {
        let level = entry.level.lowercased()
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 8) {
                Text(level.uppercased())
                    .font(.caption2.monospaced().weight(.semibold))
                    .foregroundColor(color(level))
                Text(Fmt.logTime(entry.time))
                    .font(.caption2.monospaced())
                    .foregroundColor(.secondary)
                let who = (entry.account ?? "").isEmpty ? (entry.mid ?? "") : entry.account!
                if !who.isEmpty {
                    Text(who)
                        .font(.caption2)
                        .foregroundColor(.accentColor)
                        .lineLimit(1)
                }
            }
            Text(entry.msg)
                .font(.footnote)
                .foregroundColor(level == "debug" ? .secondary : .primary)
                .textSelection(.enabled)
        }
        .padding(.vertical, 2)
    }

    private func color(_ level: String) -> Color {
        switch level {
        case "error": return .red
        case "warn": return .orange
        case "debug": return .secondary
        default: return .blue
        }
    }
}
