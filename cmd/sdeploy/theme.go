package main

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme is a palette. Every color of the app comes from one of these, so a
// new theme is one entry in themes.
type Theme struct {
	Name    string
	Dark    bool
	Bg      string // window
	Surface string // sidebar, panels, selection rows
	Overlay string // dialogs
	Border  string
	Text    string
	Muted   string
	Accent  string
	OnAcc   string // text over Accent
	Success string
	Warning string
	Danger  string
	Info    string
	AddBg   string // diff lines
	DelBg   string
}

// themes are the palettes the app ships with, by key.
var themes = map[string]Theme{
	"dracula": {Name: "Dracula", Dark: true, Bg: "#282a36", Surface: "#343746", Overlay: "#21222c", Border: "#44475a",
		Text: "#f8f8f2", Muted: "#6272a4", Accent: "#bd93f9", OnAcc: "#282a36", Success: "#50fa7b", Warning: "#f1fa8c",
		Danger: "#ff5555", Info: "#8be9fd", AddBg: "#233a2e", DelBg: "#43272f"},
	"tokyonight": {Name: "Tokyo Night", Dark: true, Bg: "#1a1b26", Surface: "#24283b", Overlay: "#16161e", Border: "#3b4261",
		Text: "#c0caf5", Muted: "#565f89", Accent: "#7aa2f7", OnAcc: "#1a1b26", Success: "#9ece6a", Warning: "#e0af68",
		Danger: "#f7768e", Info: "#7dcfff", AddBg: "#20303b", DelBg: "#37222c"},
	"tokyonight-storm": {Name: "Tokyo Night Storm", Dark: true, Bg: "#24283b", Surface: "#2f3549", Overlay: "#1f2335", Border: "#414868",
		Text: "#c0caf5", Muted: "#565f89", Accent: "#7aa2f7", OnAcc: "#1f2335", Success: "#9ece6a", Warning: "#e0af68",
		Danger: "#f7768e", Info: "#7dcfff", AddBg: "#283b4d", DelBg: "#3b2a3d"},
	"catppuccin-mocha": {Name: "Catppuccin Mocha", Dark: true, Bg: "#1e1e2e", Surface: "#313244", Overlay: "#181825", Border: "#45475a",
		Text: "#cdd6f4", Muted: "#7f849c", Accent: "#cba6f7", OnAcc: "#1e1e2e", Success: "#a6e3a1", Warning: "#f9e2af",
		Danger: "#f38ba8", Info: "#89dceb", AddBg: "#26342f", DelBg: "#3a2832"},
	"catppuccin-macchiato": {Name: "Catppuccin Macchiato", Dark: true, Bg: "#24273a", Surface: "#363a4f", Overlay: "#1e2030", Border: "#494d64",
		Text: "#cad3f5", Muted: "#8087a2", Accent: "#c6a0f6", OnAcc: "#24273a", Success: "#a6da95", Warning: "#eed49f",
		Danger: "#ed8796", Info: "#91d7e3", AddBg: "#2a3a36", DelBg: "#3c2d3b"},
	"catppuccin-frappe": {Name: "Catppuccin Frappé", Dark: true, Bg: "#303446", Surface: "#414559", Overlay: "#292c3c", Border: "#51576d",
		Text: "#c6d0f5", Muted: "#838ba7", Accent: "#ca9ee6", OnAcc: "#303446", Success: "#a6d189", Warning: "#e5c890",
		Danger: "#e78284", Info: "#99d1db", AddBg: "#34443f", DelBg: "#483640"},
	"catppuccin-latte": {Name: "Catppuccin Latte", Dark: false, Bg: "#eff1f5", Surface: "#e6e9ef", Overlay: "#dce0e8", Border: "#bcc0cc",
		Text: "#4c4f69", Muted: "#8c8fa1", Accent: "#8839ef", OnAcc: "#eff1f5", Success: "#40a02b", Warning: "#df8e1d",
		Danger: "#d20f39", Info: "#04a5e5", AddBg: "#d5ecd0", DelBg: "#f6d3da"},
	"nord": {Name: "Nord", Dark: true, Bg: "#2e3440", Surface: "#3b4252", Overlay: "#292e39", Border: "#4c566a",
		Text: "#eceff4", Muted: "#7b88a1", Accent: "#88c0d0", OnAcc: "#2e3440", Success: "#a3be8c", Warning: "#ebcb8b",
		Danger: "#bf616a", Info: "#81a1c1", AddBg: "#34443f", DelBg: "#4a3840"},
	"gruvbox-dark": {Name: "Gruvbox Dark", Dark: true, Bg: "#282828", Surface: "#3c3836", Overlay: "#1d2021", Border: "#504945",
		Text: "#ebdbb2", Muted: "#928374", Accent: "#fabd2f", OnAcc: "#282828", Success: "#b8bb26", Warning: "#fe8019",
		Danger: "#fb4934", Info: "#83a598", AddBg: "#32361a", DelBg: "#472322"},
	"gruvbox-light": {Name: "Gruvbox Light", Dark: false, Bg: "#fbf1c7", Surface: "#ebdbb2", Overlay: "#f2e5bc", Border: "#bdae93",
		Text: "#3c3836", Muted: "#928374", Accent: "#b57614", OnAcc: "#fbf1c7", Success: "#79740e", Warning: "#af3a03",
		Danger: "#9d0006", Info: "#076678", AddBg: "#e6e7a8", DelBg: "#f2c9b6"},
	"solarized-dark": {Name: "Solarized Dark", Dark: true, Bg: "#002b36", Surface: "#073642", Overlay: "#00212b", Border: "#586e75",
		Text: "#93a1a1", Muted: "#657b83", Accent: "#268bd2", OnAcc: "#fdf6e3", Success: "#859900", Warning: "#b58900",
		Danger: "#dc322f", Info: "#2aa198", AddBg: "#08403a", DelBg: "#40272a"},
	"solarized-light": {Name: "Solarized Light", Dark: false, Bg: "#fdf6e3", Surface: "#eee8d5", Overlay: "#f5eed9", Border: "#93a1a1",
		Text: "#586e75", Muted: "#93a1a1", Accent: "#268bd2", OnAcc: "#fdf6e3", Success: "#859900", Warning: "#b58900",
		Danger: "#dc322f", Info: "#2aa198", AddBg: "#dfe8c0", DelBg: "#f3d6c8"},
	"onedark": {Name: "One Dark", Dark: true, Bg: "#282c34", Surface: "#2c313a", Overlay: "#21252b", Border: "#3e4451",
		Text: "#abb2bf", Muted: "#5c6370", Accent: "#61afef", OnAcc: "#282c34", Success: "#98c379", Warning: "#e5c07b",
		Danger: "#e06c75", Info: "#56b6c2", AddBg: "#2b3a30", DelBg: "#40292d"},
	"rosepine": {Name: "Rosé Pine", Dark: true, Bg: "#191724", Surface: "#26233a", Overlay: "#1f1d2e", Border: "#403d52",
		Text: "#e0def4", Muted: "#6e6a86", Accent: "#c4a7e7", OnAcc: "#191724", Success: "#9ccfd8", Warning: "#f6c177",
		Danger: "#eb6f92", Info: "#31748f", AddBg: "#1f3238", DelBg: "#3a2234"},
	"monokai": {Name: "Monokai", Dark: true, Bg: "#272822", Surface: "#33342c", Overlay: "#1e1f1c", Border: "#49483e",
		Text: "#f8f8f2", Muted: "#75715e", Accent: "#a6e22e", OnAcc: "#272822", Success: "#a6e22e", Warning: "#e6db74",
		Danger: "#f92672", Info: "#66d9ef", AddBg: "#2f3a1f", DelBg: "#44212e"},
	"kanagawa": {Name: "Kanagawa", Dark: true, Bg: "#1f1f28", Surface: "#2a2a37", Overlay: "#16161d", Border: "#54546d",
		Text: "#dcd7ba", Muted: "#727169", Accent: "#7e9cd8", OnAcc: "#1f1f28", Success: "#98bb6c", Warning: "#e6c384",
		Danger: "#e82424", Info: "#7fb4ca", AddBg: "#2b3328", DelBg: "#43242b"},
	"everforest": {Name: "Everforest", Dark: true, Bg: "#2d353b", Surface: "#343f44", Overlay: "#232a2e", Border: "#475258",
		Text: "#d3c6aa", Muted: "#859289", Accent: "#a7c080", OnAcc: "#2d353b", Success: "#a7c080", Warning: "#dbbc7f",
		Danger: "#e67e80", Info: "#7fbbb3", AddBg: "#384b3c", DelBg: "#4c3743"},
	"github-dark": {Name: "GitHub Dark", Dark: true, Bg: "#0d1117", Surface: "#161b22", Overlay: "#010409", Border: "#30363d",
		Text: "#e6edf3", Muted: "#7d8590", Accent: "#58a6ff", OnAcc: "#0d1117", Success: "#3fb950", Warning: "#d29922",
		Danger: "#f85149", Info: "#79c0ff", AddBg: "#12261e", DelBg: "#301a1f"},
	"github-light": {Name: "GitHub Light", Dark: false, Bg: "#ffffff", Surface: "#f6f8fa", Overlay: "#eaeef2", Border: "#d0d7de",
		Text: "#1f2328", Muted: "#656d76", Accent: "#0969da", OnAcc: "#ffffff", Success: "#1a7f37", Warning: "#9a6700",
		Danger: "#cf222e", Info: "#0550ae", AddBg: "#dafbe1", DelBg: "#ffebe9"},
}

// defaultTheme is what a new install shows.
const defaultTheme = "tokyonight"

// themeKeys lists the themes in the order the picker shows them: dark ones
// first, each group by name.
func themeKeys() []string {
	keys := make([]string, 0, len(themes))
	for k := range themes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := themes[keys[i]], themes[keys[j]]
		if a.Dark != b.Dark {
			return a.Dark
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return keys
}

// lookupTheme finds a theme by key or by its display name, ignoring case,
// spaces and dashes: "Tokyo Night" and "tokyonight" are the same.
func lookupTheme(name string) (string, bool) {
	norm := func(s string) string {
		return strings.NewReplacer(" ", "", "-", "", "_", "", "é", "e").Replace(strings.ToLower(s))
	}
	want := norm(name)
	for k, t := range themes {
		if norm(k) == want || norm(t.Name) == want {
			return k, true
		}
	}
	return "", false
}

// styles are the lipgloss styles a theme produces, made once per theme.
type styles struct {
	t Theme

	app, side, panel, dialog lipgloss.Style
	text, muted, accent      lipgloss.Style
	ok, warn, bad, info      lipgloss.Style
	title, header, badge     lipgloss.Style
	sel, selDim              lipgloss.Style
	key, button, primary     lipgloss.Style
	input, inputOn           lipgloss.Style
	add, del, hunk, gutter   lipgloss.Style
}

func c(s string) lipgloss.Color { return lipgloss.Color(s) }

func newStyles(t Theme) styles {
	s := styles{t: t}
	base := lipgloss.NewStyle().Foreground(c(t.Text)).Background(c(t.Bg))
	s.app = base
	s.side = lipgloss.NewStyle().Foreground(c(t.Text)).Background(c(t.Surface))
	s.panel = base
	s.dialog = lipgloss.NewStyle().Foreground(c(t.Text)).Background(c(t.Overlay)).
		Border(lipgloss.RoundedBorder()).BorderForeground(c(t.Accent)).BorderBackground(c(t.Overlay)).Padding(1, 2)
	s.text = base
	s.muted = base.Foreground(c(t.Muted))
	s.accent = base.Foreground(c(t.Accent)).Bold(true)
	s.ok = base.Foreground(c(t.Success))
	s.warn = base.Foreground(c(t.Warning))
	s.bad = base.Foreground(c(t.Danger))
	s.info = base.Foreground(c(t.Info))
	s.title = base.Bold(true)
	s.header = base.Foreground(c(t.Muted)).Bold(true)
	s.badge = lipgloss.NewStyle().Foreground(c(t.OnAcc)).Background(c(t.Accent)).Padding(0, 1)
	s.sel = lipgloss.NewStyle().Foreground(c(t.OnAcc)).Background(c(t.Accent)).Bold(true)
	s.selDim = lipgloss.NewStyle().Foreground(c(t.Text)).Background(c(t.Border))
	s.key = base.Foreground(c(t.Accent)).Bold(true)
	s.button = lipgloss.NewStyle().Foreground(c(t.Text)).Background(c(t.Surface)).Padding(0, 1)
	s.primary = lipgloss.NewStyle().Foreground(c(t.OnAcc)).Background(c(t.Accent)).Bold(true).Padding(0, 1)
	s.input = lipgloss.NewStyle().Foreground(c(t.Text)).Background(c(t.Surface))
	s.inputOn = s.input
	s.add = lipgloss.NewStyle().Foreground(c(t.Success)).Background(c(t.AddBg))
	s.del = lipgloss.NewStyle().Foreground(c(t.Danger)).Background(c(t.DelBg))
	s.hunk = base.Foreground(c(t.Info)).Bold(true)
	s.gutter = base.Foreground(c(t.Muted))
	return s
}
