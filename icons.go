package main

import "github.com/egoist/mygo/ui"

// Icons are line drawings in the weight of SF Symbols: 24×24, a 1.75 stroke
// with round ends, painted in the color of the text around them.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" ` +
		`stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	iconFolder   = icon(`<path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H9l2 2.5h7.5A2.5 2.5 0 0 1 21 10v7.5a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5z"/>`)
	iconFolderIn = icon(`<path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H9l2 2.5h7.5A2.5 2.5 0 0 1 21 10v7.5a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5z"/><path d="M12 11v5M9.5 13.5h5"/>`)
	iconPlus     = icon(`<path d="M12 5v14M5 12h14"/>`)
	iconUpload   = icon(`<path d="M12 16V4M7 9l5-5 5 5M5 20h14"/>`)
	iconPush     = icon(`<circle cx="12" cy="12" r="9"/><path d="M12 16V8M8.5 11.5L12 8l3.5 3.5"/>`)
	iconRefresh  = icon(`<path d="M20 11a8 8 0 0 0-14.5-4.2L4 8.5M4 4v4.5h4.5M4 13a8 8 0 0 0 14.5 4.2l1.5-1.7M20 20v-4.5h-4.5"/>`)
	iconServer   = icon(`<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>`)
	iconBranch   = icon(`<circle cx="6" cy="5.5" r="2.2"/><circle cx="6" cy="18.5" r="2.2"/><circle cx="18" cy="8.5" r="2.2"/><path d="M6 7.7v8.6M18 10.7c0 4-6 3-11 6"/>`)
	iconSparkle  = icon(`<path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8zM19 16l.8 2.2L22 19l-2.2.8L19 22l-.8-2.2L16 19l2.2-.8z"/>`)
	iconGear     = icon(`<circle cx="12" cy="12" r="3"/><path d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.3 5.3l2.1 2.1M16.6 16.6l2.1 2.1M18.7 5.3l-2.1 2.1M7.4 16.6l-2.1 2.1"/>`)
	iconCheck    = icon(`<path d="M5 12.5l4.5 4.5L19 7.5"/>`)
	iconWarn     = icon(`<path d="M12 4l9 16H3z"/><path d="M12 10v4.5M12 17.5h.01"/>`)
	iconTrash    = icon(`<path d="M4 7h16M9 7V4.5h6V7M6.5 7l.8 12.5h9.4L17.5 7M10 11v5M14 11v5"/>`)
	iconPencil   = icon(`<path d="M4 20l1-4L16.5 4.5a2 2 0 0 1 3 3L8 19z"/>`)
	iconTerminal = icon(`<rect x="3" y="4.5" width="18" height="15" rx="2.5"/><path d="M7 10l3 2.5L7 15M12.5 15H17"/>`)
	iconDoc      = icon(`<path d="M7 3.5h6.5L18 8v11a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 6 19V5A1.5 1.5 0 0 1 7.5 3.5"/><path d="M13 3.5V8h5"/>`)
)

// iconLabel is the content of a button that shows an icon before its label.
func iconLabel(c *ui.Context, ic *ui.SVG, label string) func() {
	return func() {
		ui.Icon(c, ic)
		if label != "" {
			ui.Text(c, label).SingleLine()
		}
	}
}
