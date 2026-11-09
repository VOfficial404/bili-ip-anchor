package main

import "github.com/egoist/mygo/ui"

// 线条图标，24×24，颜色跟随文字。服务器 WebUI 用的是同一套。
func icon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

var (
	iconAnchor   = icon(`<circle cx="12" cy="5" r="2.5"/><path d="M12 7.5V21M7.5 11h9M4.5 14a7.5 7.5 0 0 0 15 0"/>`)
	iconOverview = icon(`<rect x="3.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.5"/>`)
	iconUsers    = icon(`<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0 1 13 0M16 4.5a3.5 3.5 0 0 1 0 7M18 14.5a6.5 6.5 0 0 1 3.5 5.5"/>`)
	iconSettings = icon(`<path d="M4 6h10M18 6h2M4 12h4M12 12h8M4 18h12M20 18h0"/><circle cx="16" cy="6" r="2"/><circle cx="10" cy="12" r="2"/><circle cx="18" cy="18" r="2"/>`)
	iconLogs     = icon(`<rect x="4" y="3" width="16" height="18" rx="2"/><path d="M8 8h8M8 12h8M8 16h5"/>`)
	iconPlus     = icon(`<path d="M12 5v14M5 12h14"/>`)
	iconPlay     = icon(`<path d="M7 5.5v13l11-6.5z"/>`)
	iconRefresh  = icon(`<path d="M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7"/>`)
	iconLogout   = icon(`<path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3M10 17l5-5-5-5M15 12H3"/>`)
	iconServer   = icon(`<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>`)
)

var (
	iconLaptop = icon(`<rect x="4" y="5" width="16" height="11" rx="1.5"/><path d="M2 19h20"/>`)
	iconPin    = icon(`<path d="M12 21s-6.5-6.2-6.5-11a6.5 6.5 0 0 1 13 0c0 4.8-6.5 11-6.5 11z"/><circle cx="12" cy="10" r="2.3"/>`)
	iconGlobe  = icon(`<circle cx="12" cy="12" r="8.5"/><path d="M3.5 12h17M12 3.5c2.5 2.6 3.5 5.4 3.5 8.5s-1 5.9-3.5 8.5c-2.5-2.6-3.5-5.4-3.5-8.5s1-5.9 3.5-8.5z"/>`)
	iconCookie = icon(`<path d="M20.5 12.5a8.5 8.5 0 1 1-9-9 3 3 0 0 0 4 4 3 3 0 0 0 5 5z"/><path d="M8.5 9.5h.01M14.5 15.5h.01M9.5 15h.01"/>`)
	iconShield = icon(`<path d="M12 3.5l7 3v5c0 4.5-3 7.8-7 9-4-1.2-7-4.5-7-9v-5z"/><path d="M9 12l2 2 4-4"/>`)
	iconTrash  = icon(`<path d="M4.5 7h15M9.5 7V4.5h5V7M6.5 7l1 13h9l1-13"/>`)
	iconImage  = icon(`<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><circle cx="9" cy="10" r="1.8"/><path d="M20.5 16l-5-5-9 8.5"/>`)
	iconFolder = icon(`<path d="M3.5 7a2 2 0 0 1 2-2h4l2 2.5h7a2 2 0 0 1 2 2V17a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z"/>`)
	iconStop   = icon(`<rect x="6" y="6" width="12" height="12" rx="2"/>`)
)
