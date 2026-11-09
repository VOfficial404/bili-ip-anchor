package main

// host 是界面之外、和所在系统有关的部分：
//   - 桌面（host_native.go）：在进程里跑 core，直接访问网络；
//   - 手机 App（host_web.go）：界面编译成 WebAssembly 在 WebView 里运行，
//     这些事交给 App 外壳里的 Go 核心（mobile/host.go），通过同源的 /host/ 接口完成。
type host interface {
	// remote 返回连接 server（已经整理过的地址）时 API 请求发往哪里
	remote(server string) (string, error)
	// startLocal 确认代理可用后在本机开始定时任务，返回连接它的 WebUI 的地址和密码。
	// 已经在运行时只更新代理。
	startLocal(proxy string) (localRun, error)
	// stopLocal 停下本机的定时任务，账号和设置都保留
	stopLocal()
	// probe 不经过服务器，直接查询经过代理时 B 站看到的出口 IP
	probe(proxy string) (zone, error)
}

type localRun struct {
	Base     string `json:"base"`
	Password string `json:"password"`
	Zone     zone   `json:"zone"`    // 经过代理时 B 站看到的出口 IP
	Warning  string `json:"warning"` // 例如代理好像没有生效
}

// picked 是选中的一张图片
type picked struct {
	name string
	data []byte
	err  error
}
