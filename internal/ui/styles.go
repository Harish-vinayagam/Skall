package ui

import "github.com/charmbracelet/lipgloss"

// SKALL colour palette — deliberate, harmonious, dark-first.
var (
	// Primary accent — a cool violet/indigo
	colorAccent    = lipgloss.Color("#7C6EF7")
	colorAccentDim = lipgloss.Color("#4B4298")

	// Status colours
	colorOnline  = lipgloss.Color("#3DDC84") // green
	colorOffline = lipgloss.Color("#FF5370") // red/pink
	colorPending = lipgloss.Color("#FFB300") // amber

	// Backgrounds
	colorBgBase  = lipgloss.Color("#0F0F1A") // near-black
	colorBgPanel = lipgloss.Color("#161625") // panel background
	colorBgSel   = lipgloss.Color("#23233A") // selected row
	colorBgInput = lipgloss.Color("#1B1B2E") // input box

	// Foregrounds
	colorFgPrimary = lipgloss.Color("#E2E0FF") // main text
	colorFgMuted   = lipgloss.Color("#6E6E8A") // secondary / timestamps
	colorFgSender  = lipgloss.Color("#A99FFF") // sender name
	colorFgSelf    = colorAccent               // own messages

	// Borders
	colorBorder      = lipgloss.Color("#2E2E50")
	colorBorderFocus = colorAccent
)

// Layout constants (set once at render time based on terminal size).
const (
	sidebarWidth = 22 // columns
	statusHeight = 1  // rows
	inputHeight  = 3  // rows (border + line)
	minChatWidth = 30
)

// --- Base styles ---

var (
	// docStyle fills the full terminal.
	docStyle = lipgloss.NewStyle().
			Background(colorBgBase)

	// panel is a bordered box used by both the sidebar and chat view.
	panelStyle = lipgloss.NewStyle().
			Background(colorBgPanel).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	panelFocusStyle = panelStyle.
			BorderForeground(colorBorderFocus)

	// --- Status bar ---
	statusBarStyle = lipgloss.NewStyle().
			Background(colorBgPanel).
			Foreground(colorFgPrimary).
			Padding(0, 1).
			Bold(true)

	statusDotOnline = lipgloss.NewStyle().
			Foreground(colorOnline).
			SetString("●")

	statusDotOffline = lipgloss.NewStyle().
				Foreground(colorOffline).
				SetString("●")

	statusDotPending = lipgloss.NewStyle().
				Foreground(colorPending).
				SetString("◌")

	statusAppName = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			SetString("SKALL")

	statusText = lipgloss.NewStyle().
			Foreground(colorFgMuted)

	// --- Sidebar ---
	sidebarTabActive = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true).
				Underline(true)

	sidebarTabInactive = lipgloss.NewStyle().
				Foreground(colorFgMuted)

	sidebarItemStyle = lipgloss.NewStyle().
				Foreground(colorFgPrimary).
				Padding(0, 1)

	sidebarItemSelected = lipgloss.NewStyle().
				Background(colorBgSel).
				Foreground(colorAccent).
				Bold(true).
				Padding(0, 1)

	sidebarItemMuted = lipgloss.NewStyle().
				Foreground(colorFgMuted).
				Padding(0, 1)

	sidebarDot = lipgloss.NewStyle().
			Foreground(colorOnline)

	// --- Chat view ---
	chatHeaderStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Padding(0, 1)

	msgTimestamp = lipgloss.NewStyle().
			Foreground(colorFgMuted)

	msgSenderSelf = lipgloss.NewStyle().
			Foreground(colorFgSelf).
			Bold(true)

	msgSenderOther = lipgloss.NewStyle().
			Foreground(colorFgSender).
			Bold(true)

	msgBodyStyle = lipgloss.NewStyle().
			Foreground(colorFgPrimary)

	msgEmptyStyle = lipgloss.NewStyle().
			Foreground(colorFgMuted).
			Italic(true).
			Padding(1, 2)

	// --- Input ---
	inputPrompt = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	inputBorder = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Background(colorBgInput).
			Padding(0, 1)

	inputBorderFocus = inputBorder.
				BorderForeground(colorBorderFocus)

	// --- Help overlay ---
	helpOverlayStyle = lipgloss.NewStyle().
				Background(colorBgPanel).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(colorBorderFocus).
				Padding(1, 3)

	helpTitleStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			MarginBottom(1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true).
			Width(16)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(colorFgPrimary)

	// --- Notification / error banner ---
	errorBannerStyle = lipgloss.NewStyle().
				Background(colorOffline).
				Foreground(colorFgPrimary).
				Padding(0, 1)
)
