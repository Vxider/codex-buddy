//go:build uconsole_gui

package uconsole

import (
	"context"
	"fmt"
	stdhtml "html"
	"image/color"
	"log"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	desktop "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xhtml "golang.org/x/net/html"

	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
	"github.com/vxider/codex-buddy/uconsole/internal/light"
)

type App struct {
	cfg         config.UConsoleConfig
	logger      *log.Logger
	fyneApp     fyne.App
	window      fyne.Window
	lightRunner *light.Runner

	stateMu                  sync.RWMutex
	servers                  []BuddyServer
	clients                  map[string]*Client
	serverSnapshots          map[string]serverSnapshot
	lastStatus               StatusResponse
	lastNotifs               []NotificationResponse
	connected                bool
	statusLine               string
	lastError                string
	lastSuccess              time.Time
	shownNotifID             string
	dialogNotifID            string
	notifDialog              dialog.Dialog
	settingsDialog           dialog.Dialog
	settingsEditor           *dialog.FormDialog
	settingsDelete           *dialog.ConfirmDialog
	continueConfirm          *dialog.ConfirmDialog
	helpDialog               dialog.Dialog
	statusUntil              time.Time
	darkMode                 bool
	selectedSettingsServerID string

	refreshMu             sync.Mutex
	refreshing            bool
	bgFill                *canvas.Rectangle
	cardFills             []*canvas.Rectangle
	cardBorders           []*canvas.Rectangle
	openSessionCardBorder *canvas.Rectangle
	sessionCardBorder     *canvas.Rectangle
	badgeFill             *canvas.Rectangle
	badgeLabel            *canvas.Text
	updatedLabel          *widget.Label
	serverStrip           *fyne.Container
	openSessionSection    *fyne.Container
	openSessionList       *fyne.Container
	sessionList           *fyne.Container
	themeButton           *widget.Button
	refreshButton         *widget.Button
	settingsButton        *widget.Button
	refreshActivity       *widget.Activity
	rootScroll            *container.Scroll
	settingsSummary       *widget.Label
	settingsList          *fyne.Container
	tailscaleState        tailscaleState
	exitNodeButton        *splitBadgeButton
	exitNodeMenu          *fyne.Container
	exitNodeMenuOpen      bool
	tailscaleBusy         bool
	openShortcutBySession map[string]fyne.KeyName
	openSessionByShortcut map[fyne.KeyName]string
	openActionPending     map[string]bool
	openHoldStop          chan struct{}
	openHoldKey           fyne.KeyName
	openHoldSessionKey    string
	openHoldProgress      float64
	shortcutRand          *rand.Rand
	scrollHoldMu          sync.Mutex
	scrollHoldStop        chan struct{}
	scrollHoldKey         fyne.KeyName
}

type serverSnapshot struct {
	Server        BuddyServer
	Status        StatusResponse
	Notifications []NotificationResponse
	Connected     bool
	Err           error
	FetchedAt     time.Time
	LastSuccess   time.Time
	FailureSince  time.Time
	RetryAfter    time.Time
}

type textSizeTheme struct {
	fyne.Theme
}

type forcedVariant struct {
	fyne.Theme
	variant fyne.ThemeVariant
}

var headerControlHeight float32 = 38

type badgeButton struct {
	widget.BaseWidget

	Text      string
	Fill      color.Color
	TextColor color.Color
	TextSize  float32
	MinWidth  float32
	Disabled  bool
	OnTapped  func()
}

type badgeButtonRenderer struct {
	button  *badgeButton
	bg      *canvas.Rectangle
	label   *canvas.Text
	objects []fyne.CanvasObject
}

type splitBadgeButton struct {
	widget.BaseWidget

	LeftText  string
	RightText string
	LeftFill  color.Color
	RightFill color.Color
	TextColor color.Color
	TextSize  float32
	MinWidth  float32
	Disabled  bool
	OnTapped  func()
}

type splitBadgeButtonRenderer struct {
	button     *splitBadgeButton
	leftCap    *canvas.Rectangle
	leftBody   *canvas.Rectangle
	rightBody  *canvas.Rectangle
	rightCap   *canvas.Rectangle
	leftLabel  *canvas.Text
	rightLabel *canvas.Text
	separator  *canvas.Line
	objects    []fyne.CanvasObject
}

type sessionListState struct {
	shortcuts      map[string]fyne.KeyName
	pending        map[string]bool
	holdSessionKey string
	holdProgress   float64
	holdLabel      string
}

type sessionGridLayout struct {
	minColumnWidth float32
	gap            float32
}

type markdownSpan struct {
	Text  string
	Style widget.RichTextStyle
}

type markdownParagraph struct {
	widget.BaseWidget
	spans []markdownSpan
}

type markdownParagraphFragment struct {
	Text  string
	Style widget.RichTextStyle
	X     float32
	Y     float32
}

var openShortcutPool = []fyne.KeyName{
	fyne.KeyB,
	fyne.KeyD,
	fyne.KeyF,
	fyne.KeyG,
	fyne.KeyH,
	fyne.KeyI,
	fyne.KeyL,
	fyne.KeyM,
	fyne.KeyO,
	fyne.KeyP,
	fyne.KeyQ,
	fyne.KeyU,
	fyne.KeyV,
	fyne.KeyW,
	fyne.KeyY,
	fyne.KeyZ,
}

func (t textSizeTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText, theme.SizeNameCaptionText:
		return 22
	default:
		return t.Theme.Size(name)
	}
}

func (f forcedVariant) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return f.Theme.Color(name, f.variant)
}

func (l sessionGridLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	visible := visibleObjects(objects)
	if len(visible) == 0 {
		return
	}

	cols := l.columns(size.Width, len(visible))
	gap := l.spacing()
	cellWidth := size.Width
	if cols > 1 {
		cellWidth = (size.Width - gap) / float32(cols)
	}
	if cellWidth < 0 {
		cellWidth = 0
	}

	x := float32(0)
	y := float32(0)
	rowHeight := float32(0)
	col := 0

	for _, obj := range visible {
		objHeight := obj.MinSize().Height
		obj.Move(fyne.NewPos(x, y))
		obj.Resize(fyne.NewSize(cellWidth, objHeight))
		if objHeight > rowHeight {
			rowHeight = objHeight
		}

		col++
		if col >= cols {
			col = 0
			x = 0
			y += rowHeight + gap
			rowHeight = 0
			continue
		}
		x += cellWidth + gap
	}
}

func (l sessionGridLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	visible := visibleObjects(objects)
	if len(visible) == 0 {
		return fyne.NewSize(0, 0)
	}

	width := float32(0)
	height := float32(0)
	gap := l.spacing()
	for i, obj := range visible {
		min := obj.MinSize()
		if min.Width > width {
			width = min.Width
		}
		height += min.Height
		if i > 0 {
			height += gap
		}
	}
	return fyne.NewSize(width, height)
}

func (l sessionGridLayout) columns(width float32, count int) int {
	if count < 2 {
		return 1
	}
	gap := l.spacing()
	minWidth := l.minWidth()
	if width >= minWidth*2+gap {
		return 2
	}
	return 1
}

func (l sessionGridLayout) minWidth() float32 {
	if l.minColumnWidth > 0 {
		return l.minColumnWidth
	}
	return 520
}

func (l sessionGridLayout) spacing() float32 {
	if l.gap > 0 {
		return l.gap
	}
	return theme.Padding()
}

func visibleObjects(objects []fyne.CanvasObject) []fyne.CanvasObject {
	visible := make([]fyne.CanvasObject, 0, len(objects))
	for _, obj := range objects {
		if obj != nil && obj.Visible() {
			visible = append(visible, obj)
		}
	}
	return visible
}

func newMarkdownParagraph(spans []markdownSpan) *markdownParagraph {
	paragraph := &markdownParagraph{spans: compactMarkdownSpans(spans)}
	paragraph.ExtendBaseWidget(paragraph)
	return paragraph
}

func compactMarkdownSpans(spans []markdownSpan) []markdownSpan {
	if len(spans) < 2 {
		return spans
	}

	merged := make([]markdownSpan, 0, len(spans))
	for _, span := range spans {
		if span.Text == "" {
			continue
		}
		lastIndex := len(merged) - 1
		if lastIndex >= 0 && merged[lastIndex].Style == span.Style {
			merged[lastIndex].Text += span.Text
			continue
		}
		merged = append(merged, span)
	}
	return merged
}

func (p *markdownParagraph) CreateRenderer() fyne.WidgetRenderer {
	return &markdownParagraphRenderer{paragraph: p}
}

type markdownParagraphRenderer struct {
	paragraph *markdownParagraph
	fragments []markdownParagraphFragment
	objects   []fyne.CanvasObject
	width     float32
	height    float32
}

func (r *markdownParagraphRenderer) Layout(size fyne.Size) {
	width := size.Width
	if width <= 0 {
		width = markdownParagraphLayoutWidth(0)
	}
	r.ensureLayout(width)

	for i, object := range r.objects {
		if i >= len(r.fragments) {
			object.Hide()
			continue
		}
		text := object.(*canvas.Text)
		fragment := r.fragments[i]
		text.Text = fragment.Text
		text.Color = markdownTextColor(fragment.Style)
		text.TextStyle = fragment.Style.TextStyle
		text.TextSize = markdownTextSize(fragment.Style)
		text.Move(fyne.NewPos(fragment.X, fragment.Y))
		text.Resize(text.MinSize())
		text.Show()
		text.Refresh()
	}
}

func (r *markdownParagraphRenderer) MinSize() fyne.Size {
	width := markdownParagraphLayoutWidth(r.paragraph.Size().Width)
	r.ensureLayout(width)
	return fyne.NewSize(width, r.height)
}

func (r *markdownParagraphRenderer) Refresh() {
	r.width = 0
	r.ensureLayout(r.paragraph.Size().Width)
	for _, object := range r.objects {
		object.Refresh()
	}
}

func (r *markdownParagraphRenderer) Objects() []fyne.CanvasObject {
	r.ensureLayout(r.paragraph.Size().Width)
	return r.objects
}

func (r *markdownParagraphRenderer) Destroy() {}

func (r *markdownParagraphRenderer) ensureLayout(width float32) {
	width = markdownParagraphLayoutWidth(width)
	if r.width == width {
		return
	}

	fragments, height := layoutMarkdownParagraph(r.paragraph.spans, width)
	r.fragments = fragments
	r.height = height
	r.width = width

	r.objects = make([]fyne.CanvasObject, 0, len(fragments))
	for _, fragment := range fragments {
		text := canvas.NewText(fragment.Text, markdownTextColor(fragment.Style))
		text.TextStyle = fragment.Style.TextStyle
		text.TextSize = markdownTextSize(fragment.Style)
		r.objects = append(r.objects, text)
	}
}

const markdownParagraphDefaultWidth float32 = 360

func markdownParagraphLayoutWidth(width float32) float32 {
	if width > 0 {
		return width
	}

	app := fyne.CurrentApp()
	if app != nil && app.Driver() != nil {
		for _, window := range app.Driver().AllWindows() {
			if window == nil || window.Canvas() == nil {
				continue
			}
			canvasSize := window.Canvas().Size()
			if canvasSize.Width <= 0 {
				continue
			}

			// Summary paragraphs live inside padded cards; reserve room for card chrome.
			hinted := canvasSize.Width - 96
			if hinted > markdownParagraphDefaultWidth {
				return hinted
			}
		}
	}

	return markdownParagraphDefaultWidth
}

func layoutMarkdownParagraph(spans []markdownSpan, width float32) ([]markdownParagraphFragment, float32) {
	width = markdownParagraphLayoutWidth(width)

	tokens := markdownTokens(spans)
	if len(tokens) == 0 {
		return nil, markdownLineHeight(widget.RichTextStyleInline)
	}

	lineSpacing := fyne.CurrentApp().Settings().Theme().Size(theme.SizeNameLineSpacing)
	fragments := make([]markdownParagraphFragment, 0, len(tokens))
	x := float32(0)
	y := float32(0)
	rowHeight := float32(0)

	newLine := func() {
		trimTrailingLineWhitespace(&fragments, y, &x)
		if rowHeight <= 0 {
			rowHeight = markdownLineHeight(widget.RichTextStyleInline)
		}
		x = 0
		y += rowHeight + lineSpacing
		rowHeight = 0
	}

	appendFragment := func(text string, style widget.RichTextStyle) {
		if text == "" {
			return
		}
		size := fyne.MeasureText(text, markdownTextSize(style), style.TextStyle)
		rowHeight = maxFloat32(rowHeight, size.Height)
		lastIndex := len(fragments) - 1
		if lastIndex >= 0 {
			last := &fragments[lastIndex]
			if last.Style == style && last.Y == y && last.X+fyne.MeasureText(last.Text, markdownTextSize(style), style.TextStyle).Width == x {
				last.Text += text
				x += size.Width
				return
			}
		}
		fragments = append(fragments, markdownParagraphFragment{
			Text:  text,
			Style: style,
			X:     x,
			Y:     y,
		})
		x += size.Width
	}

	for _, token := range tokens {
		if token.Text == "\n" {
			newLine()
			continue
		}

		text := token.Text
		style := token.Style
		for text != "" {
			if x == 0 && strings.TrimSpace(text) == "" {
				break
			}

			size := fyne.MeasureText(text, markdownTextSize(style), style.TextStyle)
			if size.Width <= width-x {
				appendFragment(text, style)
				break
			}

			if x > 0 && shouldMoveMarkdownTokenToNextLine(text, style, width-x, width) {
				newLine()
				continue
			}

			part, rest := splitMarkdownToken(text, style, width-x)
			if part != "" {
				appendFragment(part, style)
				text = rest
				if text != "" {
					newLine()
				}
				continue
			}

			if x > 0 {
				if trimTrailingLineWhitespace(&fragments, y, &x) {
					continue
				}
				newLine()
				continue
			}

			if part == "" {
				runes := []rune(text)
				part = string(runes[:1])
				rest = string(runes[1:])
			}
			appendFragment(part, style)
			text = rest
			if text != "" {
				newLine()
			}
		}
	}

	if rowHeight <= 0 {
		rowHeight = markdownLineHeight(widget.RichTextStyleInline)
	}
	return fragments, y + rowHeight
}

func trimTrailingLineWhitespace(fragments *[]markdownParagraphFragment, lineY float32, x *float32) bool {
	items := *fragments
	for i := len(items) - 1; i >= 0; i-- {
		fragment := items[i]
		if fragment.Y != lineY {
			break
		}

		trimmed := strings.TrimRightFunc(fragment.Text, unicode.IsSpace)
		if trimmed == fragment.Text {
			return false
		}

		if trimmed == "" {
			items = items[:i]
			*fragments = items
			*x = fragment.X
			return true
		}

		fragment.Text = trimmed
		items[i] = fragment
		*fragments = items
		*x = fragment.X + fyne.MeasureText(trimmed, markdownTextSize(fragment.Style), fragment.Style.TextStyle).Width
		return true
	}
	return false
}

func shouldMoveMarkdownTokenToNextLine(text string, style widget.RichTextStyle, remainingWidth float32, lineWidth float32) bool {
	if strings.TrimSpace(text) == "" || strings.Contains(text, "\n") {
		return false
	}

	fullWidth := fyne.MeasureText(text, markdownTextSize(style), style.TextStyle).Width
	if fullWidth <= remainingWidth {
		return false
	}
	if fullWidth > lineWidth {
		return false
	}

	if isInlineAccentStyle(style) {
		if isMarkdownPathLikeToken(text) && fullWidth > lineWidth*0.45 {
			return false
		}
		return true
	}
	return isMarkdownPlainWordToken(text) && fullWidth <= lineWidth*0.22
}

func isMarkdownPlainWordToken(text string) bool {
	runes := []rune(text)
	if len(runes) == 0 {
		return false
	}
	for _, r := range runes {
		if unicode.IsSpace(r) {
			return false
		}
		if r >= 0x80 {
			return false
		}
	}
	for _, r := range runes {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '-') {
			return false
		}
	}
	return true
}

func isMarkdownPathLikeToken(text string) bool {
	return strings.ContainsAny(text, "/._:")
}

func markdownTokens(spans []markdownSpan) []markdownSpan {
	tokens := make([]markdownSpan, 0, len(spans)*4)
	for _, span := range spans {
		for _, token := range splitMarkdownText(span.Text) {
			tokens = append(tokens, markdownSpan{Text: token, Style: span.Style})
		}
	}
	return tokens
}

func splitMarkdownText(text string) []string {
	tokens := make([]string, 0, len(text))
	runes := []rune(text)
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case r == '\n':
			tokens = append(tokens, "\n")
			i++
		case unicode.IsSpace(r):
			start := i
			for i < len(runes) && unicode.IsSpace(runes[i]) && runes[i] != '\n' {
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
		case r < 0x80:
			start := i
			for i < len(runes) && runes[i] < 0x80 && !unicode.IsSpace(runes[i]) {
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
		default:
			tokens = append(tokens, string(r))
			i++
		}
	}
	return tokens
}

func splitMarkdownToken(text string, style widget.RichTextStyle, width float32) (string, string) {
	if width <= 0 {
		return "", text
	}

	runes := []rune(text)
	if len(runes) <= 1 {
		return "", text
	}

	lastFit := 0
	for i := 1; i <= len(runes); i++ {
		part := string(runes[:i])
		if fyne.MeasureText(part, markdownTextSize(style), style.TextStyle).Width <= width {
			lastFit = i
			continue
		}
		break
	}
	if lastFit == 0 {
		return "", text
	}
	if preferred := preferredMarkdownBreakIndex(runes[:lastFit]); preferred > 0 && preferred < lastFit {
		lastFit = preferred
	}
	return string(runes[:lastFit]), string(runes[lastFit:])
}

func preferredMarkdownBreakIndex(runes []rune) int {
	if len(runes) < 2 {
		return 0
	}

	for i := len(runes) - 1; i > 0; i-- {
		if isMarkdownBreakRune(runes[i-1]) {
			return i
		}
	}
	return 0
}

func isMarkdownBreakRune(r rune) bool {
	switch r {
	case '/', '.', '_', '-', ':', ')', ']', ',':
		return true
	default:
		return false
	}
}

func markdownTextSize(style widget.RichTextStyle) float32 {
	name := style.SizeName
	if name == "" {
		name = theme.SizeNameText
	}
	return fyne.CurrentApp().Settings().Theme().Size(name)
}

func markdownTextColor(style widget.RichTextStyle) color.Color {
	name := style.ColorName
	if name == "" {
		name = theme.ColorNameForeground
	}
	variant := fyne.CurrentApp().Settings().ThemeVariant()
	return fyne.CurrentApp().Settings().Theme().Color(name, variant)
}

func markdownLineHeight(style widget.RichTextStyle) float32 {
	return fyne.MeasureText("Mg", markdownTextSize(style), style.TextStyle).Height
}

func maxFloat32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func newBadgeButton(text string, minWidth float32, onTapped func()) *badgeButton {
	button := &badgeButton{
		Text:      text,
		Fill:      color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF},
		TextColor: color.White,
		TextSize:  20,
		MinWidth:  minWidth,
		OnTapped:  onTapped,
	}
	button.ExtendBaseWidget(button)
	return button
}

func newSplitBadgeButton(leftText, rightText string, minWidth float32, onTapped func()) *splitBadgeButton {
	button := &splitBadgeButton{
		LeftText:  leftText,
		RightText: rightText,
		LeftFill:  color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF},
		RightFill: color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF},
		TextColor: color.White,
		TextSize:  20,
		MinWidth:  minWidth,
		OnTapped:  onTapped,
	}
	button.ExtendBaseWidget(button)
	return button
}

func (b *badgeButton) SetText(text string) {
	b.Text = text
	b.Refresh()
}

func (b *badgeButton) SetFill(fill color.Color) {
	b.Fill = fill
	b.Refresh()
}

func (b *badgeButton) Enable() {
	b.Disabled = false
	b.Refresh()
}

func (b *badgeButton) Disable() {
	b.Disabled = true
	b.Refresh()
}

func (b *badgeButton) Tapped(_ *fyne.PointEvent) {
	if b.Disabled || b.OnTapped == nil {
		return
	}
	b.OnTapped()
}

func (b *badgeButton) TappedSecondary(_ *fyne.PointEvent) {}

func (b *splitBadgeButton) SetTexts(leftText, rightText string) {
	b.LeftText = leftText
	b.RightText = rightText
	b.Refresh()
}

func (b *splitBadgeButton) SetFills(leftFill, rightFill color.Color) {
	b.LeftFill = leftFill
	b.RightFill = rightFill
	b.Refresh()
}

func (b *splitBadgeButton) Enable() {
	b.Disabled = false
	b.Refresh()
}

func (b *splitBadgeButton) Disable() {
	b.Disabled = true
	b.Refresh()
}

func (b *splitBadgeButton) Tapped(_ *fyne.PointEvent) {
	if b.Disabled || b.OnTapped == nil {
		return
	}
	b.OnTapped()
}

func (b *splitBadgeButton) TappedSecondary(_ *fyne.PointEvent) {}

func (b *badgeButton) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(b.Fill)
	bg.CornerRadius = 12

	label := canvas.NewText(b.Text, b.TextColor)
	label.Alignment = fyne.TextAlignCenter
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.TextSize = b.TextSize

	return &badgeButtonRenderer{
		button:  b,
		bg:      bg,
		label:   label,
		objects: []fyne.CanvasObject{bg, label},
	}
}

func (b *splitBadgeButton) CreateRenderer() fyne.WidgetRenderer {
	leftCap := canvas.NewRectangle(b.LeftFill)
	leftCap.CornerRadius = 12

	leftBody := canvas.NewRectangle(b.LeftFill)

	rightBody := canvas.NewRectangle(b.RightFill)

	rightCap := canvas.NewRectangle(b.RightFill)
	rightCap.CornerRadius = 12

	leftLabel := canvas.NewText(b.LeftText, b.TextColor)
	leftLabel.Alignment = fyne.TextAlignCenter
	leftLabel.TextStyle = fyne.TextStyle{Bold: true}
	leftLabel.TextSize = b.TextSize

	rightLabel := canvas.NewText(b.RightText, b.TextColor)
	rightLabel.Alignment = fyne.TextAlignCenter
	rightLabel.TextStyle = fyne.TextStyle{Bold: true}
	rightLabel.TextSize = b.TextSize

	separator := canvas.NewLine(color.NRGBA{R: 0x1B, G: 0x1E, B: 0x23, A: 0x66})
	separator.StrokeWidth = 1

	return &splitBadgeButtonRenderer{
		button:     b,
		leftCap:    leftCap,
		leftBody:   leftBody,
		rightBody:  rightBody,
		rightCap:   rightCap,
		leftLabel:  leftLabel,
		rightLabel: rightLabel,
		separator:  separator,
		objects:    []fyne.CanvasObject{leftCap, leftBody, rightBody, rightCap, separator, leftLabel, rightLabel},
	}
}

func (r *badgeButtonRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)

	labelSize := r.label.MinSize()
	r.label.Move(fyne.NewPos(
		(size.Width-labelSize.Width)/2,
		(size.Height-labelSize.Height)/2,
	))
	r.label.Resize(labelSize)
}

func (r *badgeButtonRenderer) MinSize() fyne.Size {
	width := r.button.MinWidth
	labelWidth := r.label.MinSize().Width + 28
	if labelWidth > width {
		width = labelWidth
	}
	return fyne.NewSize(width, headerControlHeight)
}

func (r *badgeButtonRenderer) Refresh() {
	r.bg.FillColor = r.button.Fill
	if r.button.Disabled {
		r.bg.FillColor = disabledBadgeFill(r.button.Fill)
	}
	r.bg.Refresh()

	r.label.Text = r.button.Text
	r.label.Color = r.button.TextColor
	if r.button.Disabled {
		r.label.Color = color.NRGBA{R: 0xE0, G: 0xE3, B: 0xE8, A: 0xD8}
	}
	r.label.TextSize = r.button.TextSize
	r.label.Refresh()

	r.Layout(r.button.Size())
}

func (r *badgeButtonRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *badgeButtonRenderer) Destroy() {}

func (r *badgeButtonRenderer) BackgroundColor() color.Color {
	return color.Transparent
}

func (r *splitBadgeButtonRenderer) Layout(size fyne.Size) {
	leftWidth := size.Width / 2
	rightWidth := size.Width - leftWidth
	corner := float32(12)

	r.leftCap.Move(fyne.NewPos(0, 0))
	r.leftCap.Resize(fyne.NewSize(corner*2, size.Height))
	r.leftBody.Move(fyne.NewPos(corner, 0))
	r.leftBody.Resize(fyne.NewSize(leftWidth-corner, size.Height))

	r.rightBody.Move(fyne.NewPos(leftWidth, 0))
	r.rightBody.Resize(fyne.NewSize(rightWidth-corner, size.Height))
	r.rightCap.Move(fyne.NewPos(size.Width-corner*2, 0))
	r.rightCap.Resize(fyne.NewSize(corner*2, size.Height))

	r.separator.Position1 = fyne.NewPos(leftWidth, 5)
	r.separator.Position2 = fyne.NewPos(leftWidth, size.Height-5)

	leftLabelSize := r.leftLabel.MinSize()
	r.leftLabel.Move(fyne.NewPos(
		(leftWidth-leftLabelSize.Width)/2,
		(size.Height-leftLabelSize.Height)/2,
	))
	r.leftLabel.Resize(leftLabelSize)

	rightLabelSize := r.rightLabel.MinSize()
	r.rightLabel.Move(fyne.NewPos(
		leftWidth+(rightWidth-rightLabelSize.Width)/2,
		(size.Height-rightLabelSize.Height)/2,
	))
	r.rightLabel.Resize(rightLabelSize)
}

func (r *splitBadgeButtonRenderer) MinSize() fyne.Size {
	leftWidth := canvas.NewText(r.button.LeftText, r.button.TextColor).MinSize().Width + 26
	rightWidth := canvas.NewText(r.button.RightText, r.button.TextColor).MinSize().Width + 24
	halfWidth := leftWidth
	if rightWidth > halfWidth {
		halfWidth = rightWidth
	}
	if halfWidth < 62 {
		halfWidth = 62
	}
	width := halfWidth * 2
	if width < r.button.MinWidth {
		width = r.button.MinWidth
	}
	return fyne.NewSize(width, headerControlHeight)
}

func (r *splitBadgeButtonRenderer) Refresh() {
	leftFill := r.button.LeftFill
	rightFill := r.button.RightFill
	if r.button.Disabled {
		leftFill = disabledBadgeFill(leftFill)
		rightFill = disabledBadgeFill(rightFill)
	}
	r.leftCap.FillColor = leftFill
	r.leftCap.Refresh()
	r.leftBody.FillColor = leftFill
	r.leftBody.Refresh()
	r.rightBody.FillColor = rightFill
	r.rightBody.Refresh()
	r.rightCap.FillColor = rightFill
	r.rightCap.Refresh()

	r.leftLabel.Text = r.button.LeftText
	r.leftLabel.Color = r.button.TextColor
	r.rightLabel.Text = r.button.RightText
	r.rightLabel.Color = r.button.TextColor
	if r.button.Disabled {
		disabledText := color.NRGBA{R: 0xE0, G: 0xE3, B: 0xE8, A: 0xD8}
		r.leftLabel.Color = disabledText
		r.rightLabel.Color = disabledText
	}
	r.leftLabel.TextSize = r.button.TextSize
	r.rightLabel.TextSize = r.button.TextSize
	r.leftLabel.Refresh()
	r.rightLabel.Refresh()
	r.separator.Refresh()

	r.Layout(r.button.Size())
}

func (r *splitBadgeButtonRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *splitBadgeButtonRenderer) Destroy() {}

func (r *splitBadgeButtonRenderer) BackgroundColor() color.Color {
	return color.Transparent
}

func Run(ctx context.Context, cfg config.UConsoleConfig, logger *log.Logger) error {
	if cfg.PollFallbackMS <= 0 || cfg.PollFallbackMS > 5000 {
		cfg.PollFallbackMS = 5000
	}
	cfg.Window.Fullscreen = false

	gui := &App{
		cfg:                   cfg,
		logger:                logger,
		clients:               make(map[string]*Client),
		serverSnapshots:       make(map[string]serverSnapshot),
		openShortcutBySession: make(map[string]fyne.KeyName),
		openSessionByShortcut: make(map[fyne.KeyName]string),
		openActionPending:     make(map[string]bool),
		shortcutRand:          rand.New(rand.NewSource(time.Now().UnixNano())),
		lastStatus:            offlineStatus(),
		statusLine:            "Connecting to codex-buddy",
		darkMode:              true,
	}

	gui.fyneApp = app.NewWithID("github.com.vxider.codex-buddy.uconsole")
	gui.applyTheme()
	gui.replaceServers(loadServers(gui.fyneApp.Preferences(), cfg.ServerURL), false)

	gui.window = gui.fyneApp.NewWindow("codex-buddy uConsole")
	gui.window.SetMaster()
	width, height := normalizedWindowSize(cfg.Window.Width, cfg.Window.Height)
	gui.window.Resize(fyne.NewSize(float32(width), float32(height)))
	gui.window.CenterOnScreen()
	gui.window.SetContent(gui.buildUI())
	gui.fyneApp.Settings().AddListener(func(_ fyne.Settings) {
		if gui.bgFill != nil {
			gui.render()
		}
	})

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	gui.window.SetCloseIntercept(func() {
		cancel()
		gui.fyneApp.Quit()
	})

	if driver, err := light.NewWS2812Driver(light.WS2812Config{
		Enabled:    cfg.LED.Enabled,
		Pixels:     cfg.LED.Pixels,
		Brightness: cfg.LED.Brightness,
		GPIOPin:    cfg.LED.GPIOPin,
		DmaNum:     cfg.LED.DmaNum,
		Frequency:  cfg.LED.Frequency,
	}, logger); err == nil {
		gui.lightRunner = light.NewRunner(light.NewDefaultStateMachine(), driver, cfg.LED.Pixels)
		gui.lightRunner.Update(gui.lastStatus.ToSnapshot(), nil)
		go func() {
			if err := gui.lightRunner.Run(childCtx); err != nil && logger != nil {
				logger.Printf("uconsole light runner failed: %v", err)
			}
		}()
	} else if logger != nil {
		logger.Printf("uconsole LED disabled: %v", err)
	}

	gui.installKeyHandlers()
	gui.render()

	go gui.startSync(childCtx)
	gui.window.ShowAndRun()
	return nil
}

func (a *App) buildUI() fyne.CanvasObject {
	darkMode := a.effectiveDarkMode()
	a.bgFill = canvas.NewRectangle(color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
	a.cardFills = nil
	a.cardBorders = nil

	a.badgeLabel = canvas.NewText("OFFLINE", color.White)
	a.badgeLabel.Alignment = fyne.TextAlignCenter
	a.badgeLabel.TextStyle = fyne.TextStyle{Bold: true}
	a.badgeLabel.TextSize = 22

	a.updatedLabel = widget.NewLabel("Updated -")
	a.updatedLabel.Wrapping = fyne.TextWrapOff
	a.updatedLabel.Truncation = fyne.TextTruncateClip
	helpHint := widget.NewLabel("? Help")
	helpHint.Wrapping = fyne.TextWrapOff
	helpHint.Truncation = fyne.TextTruncateClip

	a.settingsButton = widget.NewButtonWithIcon("Settings (S)", theme.SettingsIcon(), a.showSettingsDialog)
	a.settingsButton.Importance = widget.MediumImportance
	a.themeButton = widget.NewButtonWithIcon("Light (T)", theme.ColorPaletteIcon(), a.toggleTheme)
	a.themeButton.Importance = widget.MediumImportance
	a.refreshButton = widget.NewButtonWithIcon("Refresh (R)", theme.ViewRefreshIcon(), func() { go a.runManualRefresh() })
	a.refreshButton.Importance = widget.HighImportance
	a.refreshActivity = widget.NewActivity()
	a.refreshActivity.Hide()

	headerControlHeight = a.settingsButton.MinSize().Height
	if headerControlHeight < 38 {
		headerControlHeight = 38
	}

	a.badgeFill = canvas.NewRectangle(color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF})
	a.badgeFill.SetMinSize(fyne.NewSize(148, headerControlHeight))
	a.badgeFill.CornerRadius = 12

	a.exitNodeButton = newSplitBadgeButton("TS", "Exit", 132, a.toggleExitNodeMenu)
	a.exitNodeMenu = container.NewVBox()

	a.serverStrip = container.NewHBox(widget.NewLabel("No servers configured"))
	serverStripScroll := container.NewHScroll(a.serverStrip)
	serverStripScroll.SetMinSize(fyne.NewSize(260, headerControlHeight))

	leftGroup := container.NewHBox(
		container.NewCenter(container.NewStack(a.badgeFill, container.NewCenter(a.badgeLabel))),
		container.NewCenter(a.exitNodeButton),
	)

	rightGroup := container.NewHBox(
		a.updatedLabel,
		a.refreshActivity,
		helpHint,
		a.settingsButton,
		a.themeButton,
		a.refreshButton,
	)

	a.openSessionList = container.NewVBox()
	a.openSessionSection = container.NewVBox(
		a.sectionCard("Open Sessions", darkMode, a.openSessionList, &a.openSessionCardBorder),
	)
	a.sessionList = container.New(&sessionGridLayout{minColumnWidth: 520})

	content := container.NewVBox(
		a.openSessionSection,
		a.sectionCard("Sessions", darkMode, a.sessionList, &a.sessionCardBorder),
	)
	a.rootScroll = container.NewVScroll(content)

	header := container.NewBorder(
		nil,
		nil,
		leftGroup,
		rightGroup,
		serverStripScroll,
	)

	topBar := container.NewVBox(
		header,
		a.exitNodeMenu,
	)

	return container.NewMax(
		a.bgFill,
		container.NewPadded(
			container.NewBorder(topBar, nil, nil, nil, a.rootScroll),
		),
	)
}

func (a *App) sectionCard(title string, darkMode bool, content fyne.CanvasObject, borderSlot **canvas.Rectangle) fyne.CanvasObject {
	fill := canvas.NewRectangle(cardFill(darkMode))
	fill.CornerRadius = 18

	border := canvas.NewRectangle(color.Transparent)
	border.CornerRadius = 18
	border.StrokeWidth = 1.5
	border.StrokeColor = cardBorder(darkMode, false)

	a.cardFills = append(a.cardFills, fill)
	a.cardBorders = append(a.cardBorders, border)
	if borderSlot != nil {
		*borderSlot = border
	}

	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	body := container.NewVBox(
		titleLabel,
		widget.NewSeparator(),
		content,
	)

	return container.NewStack(
		fill,
		border,
		container.NewPadded(container.NewPadded(body)),
	)
}

func (a *App) installKeyHandlers() {
	a.window.Canvas().SetOnTypedKey(func(event *fyne.KeyEvent) {
		if a.handleModalKey(event.Name) {
			return
		}
		switch event.Name {
		case fyne.KeyA:
			go a.ackPrimary()
		case fyne.KeyC:
			a.confirmContinue()
		case fyne.KeyEscape:
			a.dismissNotification()
		case fyne.KeyJ:
			a.scrollBy(88)
			a.startScrollHold(fyne.KeyJ, 88)
		case fyne.KeyK:
			a.scrollBy(-88)
			a.startScrollHold(fyne.KeyK, -88)
		case fyne.KeyR:
			go a.runManualRefresh()
		case fyne.KeyS:
			a.showSettingsDialog()
		case fyne.KeyT:
			a.toggleTheme()
		}
	})
	a.window.Canvas().SetOnTypedRune(func(r rune) {
		if r == '?' {
			a.showHelpDialog()
		}
	})

	if deskCanvas, ok := a.window.Canvas().(desktop.Canvas); ok {
		deskCanvas.SetOnKeyDown(func(event *fyne.KeyEvent) {
			a.handleOpenShortcutKeyDown(event.Name)
		})
		deskCanvas.SetOnKeyUp(func(event *fyne.KeyEvent) {
			switch event.Name {
			case fyne.KeyJ, fyne.KeyK:
				a.stopScrollHold(event.Name)
			default:
				a.stopOpenShortcutHold(event.Name)
			}
		})
	}
}

func (a *App) handleModalKey(name fyne.KeyName) bool {
	a.stateMu.RLock()
	settingsDialog := a.settingsDialog
	settingsEditor := a.settingsEditor
	settingsDelete := a.settingsDelete
	continueConfirm := a.continueConfirm
	helpDialog := a.helpDialog
	a.stateMu.RUnlock()

	if settingsEditor != nil {
		switch name {
		case fyne.KeyEscape:
			settingsEditor.Hide()
			return true
		case fyne.KeyReturn, fyne.KeyEnter:
			settingsEditor.Submit()
			return true
		default:
			return false
		}
	}

	if settingsDelete != nil {
		switch name {
		case fyne.KeyEscape:
			settingsDelete.Hide()
			return true
		case fyne.KeyReturn, fyne.KeyEnter:
			settingsDelete.Confirm()
			return true
		default:
			return false
		}
	}

	if continueConfirm != nil {
		switch name {
		case fyne.KeyEscape:
			continueConfirm.Hide()
			return true
		case fyne.KeyReturn, fyne.KeyEnter:
			continueConfirm.Confirm()
			return true
		default:
			return false
		}
	}

	if helpDialog != nil {
		switch name {
		case fyne.KeyEscape:
			helpDialog.Hide()
			return true
		default:
			return false
		}
	}

	if settingsDialog != nil {
		switch name {
		case fyne.KeyEscape:
			settingsDialog.Hide()
			return true
		case fyne.KeyN:
			a.showServerEditor(nil)
			return true
		case fyne.KeyE:
			a.editSelectedSettingsServer()
			return true
		case fyne.KeyX:
			a.confirmDeleteSelectedSettingsServer()
			return true
		default:
			return false
		}
	}

	return false
}

func (a *App) hasBlockingDialog() bool {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.settingsDialog != nil ||
		a.settingsEditor != nil ||
		a.settingsDelete != nil ||
		a.continueConfirm != nil ||
		a.helpDialog != nil ||
		a.notifDialog != nil
}

func (a *App) handleOpenShortcutKeyDown(key fyne.KeyName) {
	if a.hasBlockingDialog() {
		return
	}

	a.stateMu.RLock()
	sessionKey, ok := a.openSessionByShortcut[key]
	if !ok || a.openActionPending[sessionKey] {
		a.stateMu.RUnlock()
		return
	}
	if a.openHoldKey == key && a.openHoldSessionKey == sessionKey {
		a.stateMu.RUnlock()
		return
	}
	session := findSessionByActionKey(a.lastStatus.Sessions, sessionKey)
	a.stateMu.RUnlock()
	if session == nil || !canContinueSession(*session) {
		return
	}

	a.startOpenShortcutHold(key, *session)
}

func (a *App) startOpenShortcutHold(key fyne.KeyName, session SessionResponse) {
	sessionKey := sessionActionKey(session)

	a.stateMu.Lock()
	if a.openHoldKey == key && a.openHoldSessionKey == sessionKey {
		a.stateMu.Unlock()
		return
	}
	if stop := a.openHoldStop; stop != nil {
		close(stop)
	}
	stop := make(chan struct{})
	a.openHoldStop = stop
	a.openHoldKey = key
	a.openHoldSessionKey = sessionKey
	a.openHoldProgress = 0
	a.stateMu.Unlock()
	fyne.Do(a.render)

	go func(stop <-chan struct{}, key fyne.KeyName, session SessionResponse, sessionKey string) {
		startedAt := time.Now()
		ticker := time.NewTicker(40 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				elapsed := time.Since(startedAt)
				progress := elapsed.Seconds() / a.openShortcutHoldDuration().Seconds()
				if progress > 1 {
					progress = 1
				}

				var shouldApprove bool
				a.stateMu.Lock()
				if a.openHoldKey != key || a.openHoldSessionKey != sessionKey {
					a.stateMu.Unlock()
					return
				}
				a.openHoldProgress = progress
				shouldApprove = progress >= 1
				a.stateMu.Unlock()
				fyne.Do(a.render)

				if !shouldApprove {
					continue
				}

				a.clearOpenShortcutHold(key, sessionKey)
				go a.executeSessionContinue(session)
				return
			}
		}
	}(stop, key, session, sessionKey)
}

func (a *App) stopOpenShortcutHold(key fyne.KeyName) {
	a.stateMu.RLock()
	sessionKey := a.openHoldSessionKey
	active := a.openHoldKey == key
	a.stateMu.RUnlock()
	if !active {
		return
	}
	a.clearOpenShortcutHold(key, sessionKey)
}

func (a *App) clearOpenShortcutHold(key fyne.KeyName, sessionKey string) {
	a.stateMu.Lock()
	if a.openHoldKey != key || a.openHoldSessionKey != sessionKey {
		a.stateMu.Unlock()
		return
	}
	if a.openHoldStop != nil {
		close(a.openHoldStop)
	}
	a.openHoldStop = nil
	a.openHoldKey = ""
	a.openHoldSessionKey = ""
	a.openHoldProgress = 0
	a.stateMu.Unlock()
	fyne.Do(a.render)
}

func (a *App) startSessionContinuePending(sessionKey string) bool {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	if a.openActionPending[sessionKey] {
		return false
	}
	a.openActionPending[sessionKey] = true
	return true
}

func (a *App) finishSessionContinuePending(sessionKey string) {
	a.stateMu.Lock()
	delete(a.openActionPending, sessionKey)
	a.stateMu.Unlock()
	fyne.Do(a.render)
}

func (a *App) openShortcutHoldDuration() time.Duration {
	if a.cfg.ContinueHoldMS <= 0 {
		return time.Second
	}
	return time.Duration(a.cfg.ContinueHoldMS) * time.Millisecond
}

func holdDurationLabel(duration time.Duration) string {
	if duration <= 0 {
		duration = time.Second
	}
	if duration%time.Second == 0 {
		return fmt.Sprintf("%ds", int(duration/time.Second))
	}
	seconds := float64(duration) / float64(time.Second)
	return fmt.Sprintf("%.1fs", seconds)
}

func (a *App) startSync(ctx context.Context) {
	a.pollLoop(ctx)
}

func (a *App) pollLoop(ctx context.Context) {
	_ = a.refreshNow(ctx, false)
	ticker := time.NewTicker(time.Duration(a.cfg.PollFallbackMS) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = a.refreshNow(ctx, false)
		}
	}
}

func (a *App) refreshNow(ctx context.Context, force bool) error {
	servers, clients, previous := a.serverTargets()
	snapshots := a.loadAllServers(ctx, servers, clients, previous, force)
	status, notifications, connected, errSummary := aggregateSnapshots(snapshots)
	tsState := loadTailscaleState(ctx)

	var err error
	if errSummary != "" {
		err = fmt.Errorf(errSummary)
	}
	a.applyState(status, notifications, snapshots, connected, err, tsState)
	return err
}

func (a *App) serverTargets() ([]BuddyServer, map[string]*Client, map[string]serverSnapshot) {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()

	servers := append([]BuddyServer(nil), a.servers...)
	clients := make(map[string]*Client, len(a.clients))
	for id, client := range a.clients {
		clients[id] = client
	}
	snapshots := make(map[string]serverSnapshot, len(a.serverSnapshots))
	for id, snapshot := range a.serverSnapshots {
		snapshots[id] = snapshot
	}
	return servers, clients, snapshots
}

func (a *App) loadAllServers(ctx context.Context, servers []BuddyServer, clients map[string]*Client, previous map[string]serverSnapshot, force bool) []serverSnapshot {
	if len(servers) == 0 {
		return nil
	}

	results := make(chan serverSnapshot, len(servers))
	var wg sync.WaitGroup
	now := time.Now()
	offlineAfter := a.serverOfflineAfter()

	for _, server := range servers {
		client := clients[server.ID]
		prev := previous[server.ID]
		wg.Add(1)
		go func(server BuddyServer, client *Client, prev serverSnapshot) {
			defer wg.Done()
			snapshot := serverSnapshot{
				Server:       server,
				Status:       offlineStatus(),
				FetchedAt:    now,
				LastSuccess:  prev.LastSuccess,
				FailureSince: prev.FailureSince,
				RetryAfter:   prev.RetryAfter,
			}
			if !force && shouldSkipServerPoll(prev) {
				snapshot.Err = fmt.Errorf("offline, press Refresh to retry")
				results <- snapshot
				return
			}
			if client == nil {
				snapshot = markServerFailure(snapshot, now, fmt.Errorf("server client unavailable"), offlineAfter)
				results <- snapshot
				return
			}

			status, err := client.LoadStatus(ctx)
			if err != nil {
				snapshot = markServerFailure(snapshot, now, err, offlineAfter)
				results <- snapshot
				return
			}

			snapshot.Connected = true
			snapshot.Status = annotateStatus(server, status)
			snapshot.LastSuccess = now
			snapshot.FailureSince = time.Time{}
			snapshot.RetryAfter = time.Time{}

			notifications, notifErr := client.LoadNotifications(ctx)
			if notifErr != nil {
				snapshot.Err = notifErr
			} else {
				snapshot.Notifications = annotateNotifications(server, notifications)
			}

			results <- snapshot
		}(server, client, prev)
	}

	wg.Wait()
	close(results)

	byID := make(map[string]serverSnapshot, len(servers))
	for snapshot := range results {
		byID[snapshot.Server.ID] = snapshot
	}

	ordered := make([]serverSnapshot, 0, len(servers))
	for _, server := range servers {
		snapshot, ok := byID[server.ID]
		if !ok {
			snapshot = serverSnapshot{
				Server:    server,
				Status:    offlineStatus(),
				FetchedAt: now,
			}
		}
		ordered = append(ordered, snapshot)
	}
	return ordered
}

func (a *App) serverOfflineAfter() time.Duration {
	duration := time.Duration(a.cfg.PollFallbackMS) * time.Millisecond * 3
	if duration < 15*time.Second {
		duration = 15 * time.Second
	}
	return duration
}

func shouldSkipServerPoll(snapshot serverSnapshot) bool {
	return !snapshot.RetryAfter.IsZero()
}

func markServerFailure(snapshot serverSnapshot, now time.Time, err error, offlineAfter time.Duration) serverSnapshot {
	snapshot.Connected = false
	snapshot.Status = offlineStatus()
	snapshot.Notifications = nil
	if snapshot.FailureSince.IsZero() {
		snapshot.FailureSince = now
	}
	if now.Sub(snapshot.FailureSince) >= offlineAfter {
		snapshot.RetryAfter = now
		err = fmt.Errorf("%s; offline, press Refresh to retry", briefError(err.Error(), 64))
	}
	snapshot.Err = err
	return snapshot
}

func annotateStatus(server BuddyServer, status StatusResponse) StatusResponse {
	status.OverallState = normalizeCompatState(status.OverallState)
	status.Sessions = annotateSessions(server, status.Sessions)
	return status
}

func annotateSessions(server BuddyServer, sessions []SessionResponse) []SessionResponse {
	if len(sessions) == 0 {
		return nil
	}
	out := make([]SessionResponse, 0, len(sessions))
	for _, session := range sessions {
		session.State = normalizeSessionState(session.State)
		session.ServerID = server.ID
		session.ServerName = server.DisplayName()
		session.ServerURL = server.BaseURL
		out = append(out, session)
	}
	return out
}

func annotateNotifications(server BuddyServer, notifications []NotificationResponse) []NotificationResponse {
	if len(notifications) == 0 {
		return nil
	}
	out := make([]NotificationResponse, 0, len(notifications))
	for _, item := range notifications {
		item.ServerID = server.ID
		item.ServerName = server.DisplayName()
		item.ServerURL = server.BaseURL
		out = append(out, item)
	}
	return out
}

func aggregateSnapshots(snapshots []serverSnapshot) (StatusResponse, []NotificationResponse, bool, string) {
	if len(snapshots) == 0 {
		status := offlineStatus()
		status.ServerTime = time.Time{}
		return status, nil, false, "No servers configured"
	}

	status := offlineStatus()
	status.ServerTime = time.Time{}

	var (
		connected      bool
		latestActivity time.Time
		sessions       []SessionResponse
		notifications  []NotificationResponse
		errors         []string
		bestRank       = -1
	)

	for _, snapshot := range snapshots {
		if snapshot.FetchedAt.After(latestActivity) {
			latestActivity = snapshot.FetchedAt
		}
		if snapshot.Err != nil {
			errors = append(errors, snapshot.Server.DisplayName()+": "+briefError(snapshot.Err.Error(), 60))
		}
		if !snapshot.Connected {
			continue
		}

		connected = true
		if rank := stateRank(snapshot.Status.OverallState); rank > bestRank {
			bestRank = rank
			status.OverallState = snapshot.Status.OverallState
			status.OverallStateDetail = snapshot.Status.OverallStateDetail
		}
		if snapshot.Status.ServerTime.After(status.ServerTime) {
			status.ServerTime = snapshot.Status.ServerTime
		}

		sessions = append(sessions, snapshot.Status.Sessions...)
		notifications = append(notifications, snapshot.Notifications...)
	}

	if status.ServerTime.IsZero() {
		status.ServerTime = latestActivity
	}

	sort.SliceStable(sessions, func(i, j int) bool {
		if !sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
		}
		if sessions[i].ServerName != sessions[j].ServerName {
			return sessions[i].ServerName < sessions[j].ServerName
		}
		return sessionListTitle(sessions[i]) < sessionListTitle(sessions[j])
	})

	sort.SliceStable(notifications, func(i, j int) bool {
		if notificationPriority(notifications[i]) != notificationPriority(notifications[j]) {
			return notificationPriority(notifications[i]) > notificationPriority(notifications[j])
		}
		if !notifications[i].UpdatedAt.Equal(notifications[j].UpdatedAt) {
			return notifications[i].UpdatedAt.After(notifications[j].UpdatedAt)
		}
		return notifications[i].ID < notifications[j].ID
	})

	status.Sessions = sessions
	status.SessionsCount = len(sessions)
	if !connected {
		status.OverallState = model.StateOffline
	}

	return status, notifications, connected, strings.Join(errors, " | ")
}

func stateRank(state model.State) int {
	state = normalizeCompatState(state)
	switch state {
	case model.StateError:
		return 5
	case model.StateAttention:
		return 4
	case model.StateRunning, model.StateRunningBash:
		return 3
	case model.StateIdle:
		return 2
	default:
		return 1
	}
}

func notificationPriority(item NotificationResponse) int {
	score := 0
	if item.State == model.NotificationPending {
		score += 100
	}
	if item.Kind == model.NotificationError {
		score += 10
	}
	return score
}

func (a *App) applyState(status StatusResponse, notifications []NotificationResponse, snapshots []serverSnapshot, connected bool, err error, tsState tailscaleState) {
	snapshotMap := make(map[string]serverSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		snapshotMap[snapshot.Server.ID] = snapshot
	}

	a.stateMu.Lock()
	a.lastStatus = status
	a.lastNotifs = notifications
	a.serverSnapshots = snapshotMap
	a.tailscaleState = tsState
	a.syncOpenSessionShortcutsLocked(status.Sessions)
	a.connected = connected
	if connected {
		a.lastSuccess = time.Now()
	}
	if err != nil {
		a.lastError = err.Error()
	} else {
		a.lastError = ""
	}
	primary := a.primaryNotificationLocked()
	if a.lightRunner != nil {
		var notification *model.NotificationSnapshot
		if primary != nil {
			item := primary.ToSnapshot()
			notification = &item
		}
		a.lightRunner.Update(status.ToSnapshot(), notification)
	}
	a.stateMu.Unlock()

	fyne.Do(func() {
		a.render()
		a.refreshSettingsDialogContent()
		a.syncNotificationDialog()
	})
}

func (a *App) primaryNotificationLocked() *NotificationResponse {
	if len(a.lastNotifs) == 0 {
		return nil
	}
	item := a.lastNotifs[0]
	return &item
}

func (a *App) render() {
	a.stateMu.RLock()
	status := a.lastStatus
	servers := append([]BuddyServer(nil), a.servers...)
	tsState := a.tailscaleState
	exitNodeMenuOpen := a.exitNodeMenuOpen
	tailscaleBusy := a.tailscaleBusy
	snapshots := make([]serverSnapshot, 0, len(servers))
	for _, server := range servers {
		if snapshot, ok := a.serverSnapshots[server.ID]; ok {
			snapshots = append(snapshots, snapshot)
			continue
		}
		snapshots = append(snapshots, serverSnapshot{
			Server:    server,
			Status:    offlineStatus(),
			FetchedAt: time.Time{},
		})
	}
	darkMode := a.effectiveDarkMode()
	a.stateMu.RUnlock()

	badgeText, badgeColor := badgeStyle(status.OverallState)
	a.badgeLabel.Text = strings.ToUpper(badgeText)
	a.badgeLabel.Color = color.White
	a.badgeLabel.Refresh()
	a.badgeFill.FillColor = badgeColor
	a.badgeFill.Refresh()

	a.bgFill.FillColor = appBackground(darkMode)
	a.bgFill.Refresh()
	for _, fill := range a.cardFills {
		fill.FillColor = cardFill(darkMode)
		fill.Refresh()
	}
	for _, border := range a.cardBorders {
		border.StrokeColor = cardBorder(darkMode, false)
		border.Refresh()
	}
	openSessions, otherSessions := splitSessionsByOpenState(status.Sessions)
	if a.openSessionCardBorder != nil {
		a.openSessionCardBorder.StrokeColor = cardBorder(darkMode, len(openSessions) > 0)
		a.openSessionCardBorder.Refresh()
	}
	if a.sessionCardBorder != nil {
		a.sessionCardBorder.StrokeColor = cardBorder(darkMode, status.OverallState == model.StateError)
		a.sessionCardBorder.Refresh()
	}

	a.themeButton.SetText(themeToggleLabel(darkMode))
	if status.ServerTime.IsZero() {
		a.updatedLabel.SetText("Updated -")
	} else {
		a.updatedLabel.SetText("Updated " + status.ServerTime.Local().Format("15:04:05"))
	}
	a.renderTailscale(tsState, darkMode, exitNodeMenuOpen, tailscaleBusy)

	a.renderServerStrip(snapshots, darkMode)
	a.renderSessionList(openSessions, otherSessions, darkMode)
}

func (a *App) renderServerStrip(snapshots []serverSnapshot, darkMode bool) {
	if a.serverStrip == nil {
		return
	}
	a.serverStrip.Objects = serverStripObjects(snapshots, darkMode)
	a.serverStrip.Refresh()
}

func (a *App) renderSessionList(openSessions, otherSessions []SessionResponse, darkMode bool) {
	a.stateMu.RLock()
	view := sessionListState{
		shortcuts:      cloneShortcutMap(a.openShortcutBySession),
		pending:        clonePendingMap(a.openActionPending),
		holdSessionKey: a.openHoldSessionKey,
		holdProgress:   a.openHoldProgress,
		holdLabel:      holdDurationLabel(a.openShortcutHoldDuration()),
	}
	a.stateMu.RUnlock()

	if a.openSessionSection != nil && a.openSessionList != nil {
		if len(openSessions) == 0 {
			a.openSessionSection.Hide()
		} else {
			a.openSessionList.Objects = openSessionListObjects(openSessions, darkMode, view)
			a.openSessionList.Refresh()
			a.openSessionSection.Show()
			a.openSessionSection.Refresh()
		}
	}

	a.sessionList.Objects = sessionListObjects(otherSessions, len(openSessions) > 0, darkMode)
	a.sessionList.Refresh()
}

func (a *App) runManualRefresh() {
	a.refreshMu.Lock()
	if a.refreshing {
		a.refreshMu.Unlock()
		return
	}
	a.refreshing = true
	a.refreshMu.Unlock()

	fyne.Do(func() {
		a.refreshButton.Disable()
		a.refreshActivity.Show()
		a.refreshActivity.Start()
	})

	defer func() {
		fyne.Do(func() {
			a.refreshActivity.Stop()
			a.refreshActivity.Hide()
			a.refreshButton.Enable()
		})

		a.refreshMu.Lock()
		a.refreshing = false
		a.refreshMu.Unlock()
	}()

	_ = a.refreshNow(context.Background(), true)
}

func (a *App) ackPrimary() {
	a.stateMu.RLock()
	primary := a.primaryNotificationLocked()
	a.stateMu.RUnlock()
	if primary == nil {
		return
	}
	go a.ackNotification(*primary)
}

func (a *App) confirmContinue() {
	a.stateMu.RLock()
	primary := a.primaryNotificationLocked()
	a.stateMu.RUnlock()
	if primary == nil || !canContinue(*primary) {
		return
	}

	a.window.RequestFocus()
	confirm := dialog.NewConfirm("Send Continue", "Send one \"continue + Enter\" action to the current Codex session?", func(ok bool) {
		if ok {
			go a.executeContinue(*primary)
		}
	}, a.window)
	confirm.SetConfirmText("Send (Enter)")
	confirm.SetDismissText("Cancel (Esc)")
	confirm.SetOnClosed(func() {
		a.stateMu.Lock()
		if a.continueConfirm == confirm {
			a.continueConfirm = nil
		}
		a.stateMu.Unlock()
	})
	a.stateMu.Lock()
	a.continueConfirm = confirm
	a.stateMu.Unlock()
	confirm.Show()
}

func (a *App) executeContinue(item NotificationResponse) {
	client := a.clientForServer(item.ServerID)
	if client == nil {
		a.setStatusLine("Selected server is unavailable")
		return
	}
	a.setStatusLine("Sending continue...")
	if err := client.ContinueNotification(context.Background(), item); err != nil {
		a.setStatusLine(err.Error())
		return
	}
	a.setStatusLine("Continue sent")
	_ = a.refreshNow(context.Background(), true)
}

func (a *App) executeSessionContinue(session SessionResponse) {
	sessionKey := sessionActionKey(session)
	if !a.startSessionContinuePending(sessionKey) {
		return
	}
	defer a.finishSessionContinuePending(sessionKey)

	client := a.clientForServer(session.ServerID)
	if client == nil {
		a.setStatusLine("Selected server is unavailable")
		return
	}

	a.setStatusLine("Approving " + sessionListTitle(session) + "...")
	if err := client.ContinueSession(context.Background(), session); err != nil {
		a.setStatusLine(err.Error())
		return
	}
	a.setStatusLine("Approve sent")
	_ = a.refreshNow(context.Background(), true)
}

func (a *App) ackNotification(item NotificationResponse) {
	if strings.TrimSpace(item.ID) == "" {
		return
	}
	client := a.clientForServer(item.ServerID)
	if client == nil {
		a.setStatusLine("Selected server is unavailable")
		return
	}
	if err := client.AckNotification(context.Background(), item.ID); err != nil {
		a.setStatusLine(err.Error())
		return
	}
	_ = a.refreshNow(context.Background(), true)
}

func (a *App) clientForServer(serverID string) *Client {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.clients[serverID]
}

func (a *App) setStatusLine(message string) {
	a.stateMu.Lock()
	a.statusLine = message
	a.statusUntil = time.Now().Add(4 * time.Second)
	a.stateMu.Unlock()
	fyne.Do(a.render)

	time.AfterFunc(4*time.Second, func() {
		a.stateMu.Lock()
		if !a.statusUntil.IsZero() && time.Now().After(a.statusUntil) {
			a.statusLine = ""
		}
		a.stateMu.Unlock()
		fyne.Do(a.render)
	})
}

func (a *App) syncNotificationDialog() {
	a.stateMu.RLock()
	primary := a.primaryNotificationLocked()
	status := a.lastStatus
	currentDialog := a.notifDialog
	currentDialogID := a.dialogNotifID
	shownID := a.shownNotifID
	a.stateMu.RUnlock()

	if primary == nil || !shouldPopupNotification(*primary) {
		if currentDialog != nil {
			currentDialog.Hide()
		}
		return
	}

	key := notificationKey(*primary)
	if currentDialog != nil && currentDialogID == key {
		return
	}
	if shownID == key {
		return
	}
	if currentDialog != nil {
		currentDialog.Hide()
	}

	item := *primary
	sessionLabel := sessionLabelByID(status.Sessions, item.ServerID, item.SessionID)
	dlg := a.buildNotificationDialog(item, sessionLabel)

	a.stateMu.Lock()
	a.notifDialog = dlg
	a.dialogNotifID = key
	a.shownNotifID = key
	a.stateMu.Unlock()

	a.window.RequestFocus()
	dlg.Show()
}

func notificationKey(item NotificationResponse) string {
	return item.ServerID + ":" + item.ID
}

func (a *App) buildNotificationDialog(item NotificationResponse, sessionLabel string) dialog.Dialog {
	title := widget.NewLabelWithStyle(cardTitle(&item), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	session := widget.NewLabel("Session: " + valueOrDash(sessionLabel))
	session.Wrapping = fyne.TextWrapWord
	server := widget.NewLabel("Server: " + valueOrDash(item.ServerName))
	server.Wrapping = fyne.TextWrapWord
	summary := widget.NewLabel(valueOrDash(item.Summary))
	summary.Wrapping = fyne.TextWrapWord
	summary.TextStyle = fyne.TextStyle{Bold: true}
	meta := widget.NewLabel(primaryMeta(item))
	meta.Wrapping = fyne.TextWrapWord

	var dlg dialog.Dialog
	ackBtn := widget.NewButton("Acknowledge (A)", func() {
		if dlg != nil {
			dlg.Hide()
		}
		go a.ackNotification(item)
	})
	ackBtn.Importance = widget.HighImportance

	continueBtn := widget.NewButton("Continue (C)", func() {
		if dlg != nil {
			dlg.Hide()
		}
		a.confirmContinueItem(item)
	})
	continueBtn.Importance = widget.HighImportance
	if !canContinue(item) {
		continueBtn.Disable()
	}

	content := container.NewVBox(
		title,
		session,
		server,
		widget.NewSeparator(),
		summary,
		meta,
		container.NewGridWithColumns(2, ackBtn, continueBtn),
	)

	dlg = dialog.NewCustom(cardTitle(&item), "Dismiss (Esc)", content, a.window)
	dlg.SetOnClosed(func() {
		a.stateMu.Lock()
		defer a.stateMu.Unlock()
		if a.notifDialog == dlg {
			a.notifDialog = nil
			a.dialogNotifID = ""
		}
	})
	return dlg
}

func (a *App) confirmContinueItem(item NotificationResponse) {
	if !canContinue(item) {
		return
	}
	a.window.RequestFocus()
	confirm := dialog.NewConfirm("Send Continue", "Send one \"continue + Enter\" action to the current Codex session?", func(ok bool) {
		if ok {
			go a.executeContinue(item)
		}
	}, a.window)
	confirm.SetConfirmText("Send (Enter)")
	confirm.SetDismissText("Cancel (Esc)")
	confirm.SetOnClosed(func() {
		a.stateMu.Lock()
		if a.continueConfirm == confirm {
			a.continueConfirm = nil
		}
		a.stateMu.Unlock()
	})
	a.stateMu.Lock()
	a.continueConfirm = confirm
	a.stateMu.Unlock()
	confirm.Show()
}

func (a *App) showSettingsDialog() {
	a.stateMu.RLock()
	currentDialog := a.settingsDialog
	a.stateMu.RUnlock()
	if currentDialog != nil {
		a.refreshSettingsDialogContent()
		currentDialog.Show()
		return
	}

	a.settingsSummary = widget.NewLabel("")
	a.settingsSummary.Wrapping = fyne.TextWrapWord
	a.settingsList = container.NewVBox()

	addButton := widget.NewButtonWithIcon("Add Server (N)", theme.ContentAddIcon(), func() {
		a.showServerEditor(nil)
	})
	addButton.Importance = widget.HighImportance

	body := container.NewBorder(
		container.NewVBox(
			a.settingsSummary,
			widget.NewSeparator(),
			addButton,
			widget.NewSeparator(),
		),
		nil,
		nil,
		nil,
		container.NewVScroll(a.settingsList),
	)
	body.Resize(fyne.NewSize(760, 520))

	dlg := dialog.NewCustom("Servers", "Close (Esc)", body, a.window)
	dlg.Resize(fyne.NewSize(760, 520))
	dlg.SetOnClosed(func() {
		a.stateMu.Lock()
		defer a.stateMu.Unlock()
		if a.settingsDialog == dlg {
			a.settingsDialog = nil
			a.settingsSummary = nil
			a.settingsList = nil
		}
	})

	a.stateMu.Lock()
	if a.selectedSettingsServerID == "" && len(a.servers) > 0 {
		a.selectedSettingsServerID = a.servers[0].ID
	}
	a.settingsDialog = dlg
	a.stateMu.Unlock()

	a.refreshSettingsDialogContent()
	dlg.Show()
}

func (a *App) refreshSettingsDialogContent() {
	a.stateMu.RLock()
	if a.settingsDialog == nil || a.settingsSummary == nil || a.settingsList == nil {
		a.stateMu.RUnlock()
		return
	}
	servers := append([]BuddyServer(nil), a.servers...)
	selectedID := a.selectedSettingsServerID
	snapshots := make([]serverSnapshot, 0, len(servers))
	for _, server := range servers {
		if snapshot, ok := a.serverSnapshots[server.ID]; ok {
			snapshots = append(snapshots, snapshot)
			continue
		}
		snapshots = append(snapshots, serverSnapshot{
			Server: server,
			Status: offlineStatus(),
		})
	}
	darkMode := a.effectiveDarkMode()
	dialogRef := a.settingsDialog
	a.stateMu.RUnlock()

	if selectedID == "" && len(servers) > 0 {
		selectedID = servers[0].ID
		a.stateMu.Lock()
		if a.selectedSettingsServerID == "" {
			a.selectedSettingsServerID = selectedID
		}
		a.stateMu.Unlock()
	}

	fyne.Do(func() {
		if a.settingsSummary == nil || a.settingsList == nil {
			return
		}
		if len(servers) == 0 {
			a.settingsSummary.SetText("No servers configured. Add at least one Codex Buddy server URL.")
		} else {
			a.settingsSummary.SetText(fmt.Sprintf("%d server%s configured. Main view aggregates status and sessions across all of them.", len(servers), pluralSuffix(len(servers))))
		}
		a.settingsList.Objects = settingsServerRows(a, snapshots, darkMode, selectedID)
		a.settingsList.Refresh()
		if dialogRef != nil {
			dialogRef.Refresh()
		}
	})
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func (a *App) showServerEditor(editing *BuddyServer) {
	nameEntry := widget.NewEntry()
	urlEntry := widget.NewEntry()
	urlEntry.Validator = func(value string) error {
		_, err := normalizedBaseURL(value)
		return err
	}

	if editing != nil {
		nameEntry.SetText(editing.Name)
		urlEntry.SetText(editing.BaseURL)
	} else {
		urlEntry.SetPlaceHolder("http://127.0.0.1:8787")
	}

	items := []*widget.FormItem{
		widget.NewFormItem("Name", nameEntry),
		widget.NewFormItem("Server URL", urlEntry),
	}

	title := "Add Server"
	if editing != nil {
		title = "Edit Server"
	}

	formDialog := dialog.NewForm(title, "Save (Enter)", "Cancel (Esc)", items, func(ok bool) {
		if !ok {
			return
		}
		server := BuddyServer{}
		if editing != nil {
			server.ID = editing.ID
		}
		server.Name = nameEntry.Text
		server.BaseURL = urlEntry.Text
		if err := a.upsertServer(server); err != nil {
			dialog.ShowError(err, a.window)
			return
		}
		go a.runManualRefresh()
	}, a.window)
	formDialog.Resize(fyne.NewSize(760, 320))
	formDialog.SetOnClosed(func() {
		a.stateMu.Lock()
		if a.settingsEditor == formDialog {
			a.settingsEditor = nil
		}
		a.stateMu.Unlock()
	})
	a.stateMu.Lock()
	a.settingsEditor = formDialog
	a.stateMu.Unlock()
	formDialog.Show()
}

func (a *App) upsertServer(server BuddyServer) error {
	baseURL, err := normalizedBaseURL(server.BaseURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(server.ID) == "" {
		server.ID = newServerID()
	}
	server.BaseURL = baseURL
	server.Name = normalizedServerName(server.Name, baseURL)

	a.stateMu.RLock()
	servers := append([]BuddyServer(nil), a.servers...)
	a.stateMu.RUnlock()

	replaced := false
	for i := range servers {
		if servers[i].ID == server.ID {
			servers[i] = server
			replaced = true
			break
		}
	}
	if !replaced {
		servers = append(servers, server)
	}

	a.replaceServers(servers, true)
	return nil
}

func (a *App) deleteServer(server BuddyServer) {
	a.stateMu.RLock()
	servers := append([]BuddyServer(nil), a.servers...)
	a.stateMu.RUnlock()

	filtered := make([]BuddyServer, 0, len(servers))
	for _, item := range servers {
		if item.ID != server.ID {
			filtered = append(filtered, item)
		}
	}
	a.replaceServers(filtered, true)
	go a.runManualRefresh()
}

func (a *App) replaceServers(servers []BuddyServer, persist bool) {
	servers = sanitizeServers(servers)

	a.stateMu.Lock()
	oldClients := a.clients
	oldSnapshots := a.serverSnapshots

	clients := make(map[string]*Client, len(servers))
	snapshots := make(map[string]serverSnapshot, len(servers))
	for _, server := range servers {
		client, ok := oldClients[server.ID]
		if !ok || client == nil || client.baseURL != server.BaseURL {
			client = NewClient(server.BaseURL, time.Duration(a.cfg.HTTPTimeoutMS)*time.Millisecond, a.logger)
		}
		clients[server.ID] = client

		snapshot, ok := oldSnapshots[server.ID]
		if !ok {
			snapshot = serverSnapshot{
				Server:    server,
				Status:    offlineStatus(),
				FetchedAt: time.Time{},
			}
		}
		snapshot.Server = server
		snapshots[server.ID] = snapshot
	}

	a.servers = append([]BuddyServer(nil), servers...)
	a.clients = clients
	a.serverSnapshots = snapshots
	if len(servers) == 0 {
		a.selectedSettingsServerID = ""
	} else {
		selectedFound := false
		for _, server := range servers {
			if server.ID == a.selectedSettingsServerID {
				selectedFound = true
				break
			}
		}
		if !selectedFound {
			a.selectedSettingsServerID = servers[0].ID
		}
	}

	orderedSnapshots := make([]serverSnapshot, 0, len(servers))
	for _, server := range servers {
		orderedSnapshots = append(orderedSnapshots, snapshots[server.ID])
	}
	a.lastStatus, a.lastNotifs, a.connected, a.lastError = aggregateSnapshots(orderedSnapshots)
	a.syncOpenSessionShortcutsLocked(a.lastStatus.Sessions)
	a.stateMu.Unlock()

	if persist && a.fyneApp != nil {
		saveServers(a.fyneApp.Preferences(), servers)
	}

	fyne.Do(func() {
		if a.bgFill != nil {
			a.render()
		}
		a.refreshSettingsDialogContent()
	})
}

func (a *App) syncOpenSessionShortcutsLocked(sessions []SessionResponse) {
	activeSessions := make(map[string]bool)
	nextBySession := make(map[string]fyne.KeyName)
	usedKeys := make(map[fyne.KeyName]bool)

	for _, session := range sessions {
		if !isOpenSession(session) {
			continue
		}
		sessionKey := sessionActionKey(session)
		activeSessions[sessionKey] = true
		if key, ok := a.openShortcutBySession[sessionKey]; ok && key != "" && !usedKeys[key] {
			nextBySession[sessionKey] = key
			usedKeys[key] = true
		}
	}

	available := shuffledShortcutKeys(a.shortcutRand, usedKeys)
	for _, session := range sessions {
		if !isOpenSession(session) {
			continue
		}
		sessionKey := sessionActionKey(session)
		if _, ok := nextBySession[sessionKey]; ok {
			continue
		}
		if len(available) == 0 {
			break
		}
		nextBySession[sessionKey] = available[0]
		usedKeys[available[0]] = true
		available = available[1:]
	}

	nextByShortcut := make(map[fyne.KeyName]string, len(nextBySession))
	for sessionKey, key := range nextBySession {
		nextByShortcut[key] = sessionKey
	}

	if a.openHoldSessionKey != "" {
		assignedKey, ok := nextBySession[a.openHoldSessionKey]
		if !ok || assignedKey != a.openHoldKey {
			if a.openHoldStop != nil {
				close(a.openHoldStop)
			}
			a.openHoldStop = nil
			a.openHoldKey = ""
			a.openHoldSessionKey = ""
			a.openHoldProgress = 0
		}
	}

	for sessionKey := range a.openActionPending {
		if !activeSessions[sessionKey] {
			delete(a.openActionPending, sessionKey)
		}
	}

	a.openShortcutBySession = nextBySession
	a.openSessionByShortcut = nextByShortcut
}

func shuffledShortcutKeys(rng *rand.Rand, used map[fyne.KeyName]bool) []fyne.KeyName {
	keys := make([]fyne.KeyName, 0, len(openShortcutPool))
	for _, key := range openShortcutPool {
		if used[key] {
			continue
		}
		keys = append(keys, key)
	}
	if rng == nil {
		return keys
	}
	rng.Shuffle(len(keys), func(i, j int) {
		keys[i], keys[j] = keys[j], keys[i]
	})
	return keys
}

func sanitizeServers(servers []BuddyServer) []BuddyServer {
	out := make([]BuddyServer, 0, len(servers))
	seen := make(map[string]int, len(servers))
	for _, server := range servers {
		baseURL, err := normalizedBaseURL(server.BaseURL)
		if err != nil {
			continue
		}
		id := strings.TrimSpace(server.ID)
		if id == "" {
			id = newServerID()
		}
		item := BuddyServer{
			ID:      id,
			Name:    normalizedServerName(server.Name, baseURL),
			BaseURL: baseURL,
		}
		if index, ok := seen[item.BaseURL]; ok {
			out[index] = item
			continue
		}
		seen[item.BaseURL] = len(out)
		out = append(out, item)
	}
	return out
}

func offlineStatus() StatusResponse {
	return StatusResponse{
		OverallState:  model.StateOffline,
		SessionsCount: 0,
		Sessions:      nil,
		ServerTime:    time.Time{},
	}
}

func badgeStyle(state model.State) (string, color.NRGBA) {
	state = normalizeCompatState(state)
	switch state {
	case model.StateIdle:
		return "idle", color.NRGBA{R: 0x58, G: 0x72, B: 0x58, A: 0xFF}
	case model.StateRunning, model.StateRunningBash:
		return "running", color.NRGBA{R: 0x19, G: 0x5F, B: 0x92, A: 0xFF}
	case model.StateAttention:
		return "open", color.NRGBA{R: 0xBC, G: 0x7A, B: 0x00, A: 0xFF}
	case model.StateError:
		return "error", color.NRGBA{R: 0xB6, G: 0x3B, B: 0x2F, A: 0xFF}
	default:
		return "offline", color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF}
	}
}

func titleForState(state model.State) string {
	state = normalizeCompatState(state)
	switch state {
	case model.StateAttention:
		return "Open"
	case model.StateError:
		return "This run hit an error"
	case model.StateRunning, model.StateRunningBash:
		return "Codex is working"
	case model.StateIdle:
		return "Currently idle"
	default:
		return "Codex companion"
	}
}

func summaryForState(state model.State, connected bool, lastError string) string {
	state = normalizeCompatState(state)
	if !connected && lastError != "" {
		return "Connection to the remote buddy is unavailable: " + lastError
	}
	switch state {
	case model.StateAttention:
		return "A run just finished. It is open for a follow-up step."
	case model.StateError:
		return "An error notification will pop up when intervention is needed."
	case model.StateRunning, model.StateRunningBash:
		return "Remote session is active. Alerts and quick actions stay available here."
	case model.StateIdle:
		return "There is no active task right now."
	default:
		return "Waiting for remote codex-buddy state."
	}
}

func sessionBadgeStyle(state model.State) (string, color.NRGBA) {
	state = normalizeSessionState(state)
	switch state {
	case model.StateIdle:
		return "idle", color.NRGBA{R: 0x58, G: 0x72, B: 0x58, A: 0xFF}
	case model.StateRunning, model.StateRunningBash:
		return "running", color.NRGBA{R: 0x19, G: 0x5F, B: 0x92, A: 0xFF}
	case model.StateAttention:
		return "open", color.NRGBA{R: 0xBC, G: 0x7A, B: 0x00, A: 0xFF}
	case model.StateError:
		return "error", color.NRGBA{R: 0xB6, G: 0x3B, B: 0x2F, A: 0xFF}
	default:
		return "idle", color.NRGBA{R: 0x58, G: 0x72, B: 0x58, A: 0xFF}
	}
}
func cardTitle(primary *NotificationResponse) string {
	if primary == nil {
		return "Primary Card"
	}
	switch primary.Kind {
	case model.NotificationError:
		return "Error Card"
	default:
		return "Open Card"
	}
}

func primaryMeta(primary NotificationResponse) string {
	if primary.Kind == model.NotificationError {
		return "Error notification: resolve it from the terminal."
	}
	if primary.State == model.NotificationAcked {
		return "This notification is acknowledged, but continue is still available."
	}
	return "Press A to acknowledge, C to continue, or Esc to dismiss."
}

func serverListObjects(snapshots []serverSnapshot, darkMode bool) []fyne.CanvasObject {
	if len(snapshots) == 0 {
		return []fyne.CanvasObject{widget.NewLabel("No servers configured")}
	}

	objects := make([]fyne.CanvasObject, 0, len(snapshots)*2)
	for i, snapshot := range snapshots {
		objects = append(objects, serverRow(snapshot, darkMode))
		if i < len(snapshots)-1 {
			objects = append(objects, widget.NewSeparator())
		}
	}
	return objects
}

func serverStripObjects(snapshots []serverSnapshot, darkMode bool) []fyne.CanvasObject {
	if len(snapshots) == 0 {
		return []fyne.CanvasObject{serverStatusChip("No servers", false, darkMode)}
	}

	objects := make([]fyne.CanvasObject, 0, len(snapshots))
	for _, snapshot := range snapshots {
		objects = append(objects, serverStatusChip(snapshot.Server.DisplayName(), snapshot.Connected, darkMode))
	}
	return objects
}

func splitSessionsByOpenState(sessions []SessionResponse) ([]SessionResponse, []SessionResponse) {
	openSessions := make([]SessionResponse, 0, len(sessions))
	otherSessions := make([]SessionResponse, 0, len(sessions))
	for _, session := range sessions {
		if isOpenSession(session) {
			openSessions = append(openSessions, session)
			continue
		}
		otherSessions = append(otherSessions, session)
	}
	return openSessions, otherSessions
}

func openSessionListObjects(sessions []SessionResponse, darkMode bool, state sessionListState) []fyne.CanvasObject {
	objects := []fyne.CanvasObject{metaText("Hold the shown letter to approve.", darkMode)}
	if len(sessions) > 0 {
		objects = append(objects, widget.NewSeparator())
	}
	for i, session := range sessions {
		objects = append(objects, container.NewPadded(openSessionRow(session, darkMode, state)))
		if i < len(sessions)-1 {
			objects = append(objects, widget.NewSeparator())
		}
	}
	return objects
}

func sessionListObjects(sessions []SessionResponse, hasOpenSessions bool, darkMode bool) []fyne.CanvasObject {
	if len(sessions) == 0 {
		if hasOpenSessions {
			return []fyne.CanvasObject{widget.NewLabel("No other sessions")}
		}
		return []fyne.CanvasObject{widget.NewLabel("No active sessions")}
	}

	objects := make([]fyne.CanvasObject, 0, len(sessions))
	for _, session := range sessions {
		objects = append(objects, sessionCell(session, darkMode))
	}
	return objects
}

func openSessionRow(session SessionResponse, darkMode bool, state sessionListState) fyne.CanvasObject {
	sessionKey := sessionActionKey(session)
	shortcutKey := state.shortcuts[sessionKey]
	isPending := state.pending[sessionKey]
	isHolding := state.holdSessionKey == sessionKey && state.holdProgress > 0

	title := widget.NewLabelWithStyle(sessionListTitle(session), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Wrapping = fyne.TextWrapOff
	title.Truncation = fyne.TextTruncateEllipsis

	badges := container.NewHBox(
		sourceBadge(session.ServerName, darkMode),
		openShortcutBadge(shortcutKey, isHolding, isPending),
	)

	header := container.NewBorder(nil, nil, nil, badges, title)

	summary := sessionSummaryObject(session, darkMode, "Waiting for approval")

	hintText := "No shortcut available"
	switch {
	case isPending:
		hintText = "Approving..."
	case shortcutKey != "":
		hintText = "Hold " + strings.ToUpper(string(shortcutKey)) + " for " + state.holdLabel + " to approve"
	}
	if !canContinueSession(session) && !isPending {
		hintText = "Continue is unavailable"
	}

	objects := []fyne.CanvasObject{
		header,
		summary,
		container.NewHBox(
			metaText("Updated "+relativeTime(session.UpdatedAt), darkMode),
			layout.NewSpacer(),
			metaText(hintText, darkMode),
		),
	}

	if isHolding {
		progress := widget.NewProgressBar()
		progress.TextFormatter = func() string { return "" }
		progress.SetValue(state.holdProgress)
		objects = append(objects, progress)
	}

	return container.NewVBox(objects...)
}

func openShortcutBadge(key fyne.KeyName, active bool, pending bool) fyne.CanvasObject {
	label := "-"
	fill := color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF}
	textColor := color.Color(color.White)

	switch {
	case pending:
		label = "..."
		fill = color.NRGBA{R: 0x2D, G: 0x7A, B: 0x52, A: 0xFF}
	case key != "":
		label = strings.ToUpper(string(key))
		if active {
			fill = color.NRGBA{R: 0xBC, G: 0x7A, B: 0x00, A: 0xFF}
		}
	default:
		textColor = color.NRGBA{R: 0xCF, G: 0xD4, B: 0xDB, A: 0xFF}
	}

	bg := canvas.NewRectangle(fill)
	bg.SetMinSize(fyne.NewSize(54, 32))
	bg.CornerRadius = 10

	text := canvas.NewText(label, textColor)
	text.Alignment = fyne.TextAlignCenter
	text.TextStyle = fyne.TextStyle{Bold: true}
	text.TextSize = 22

	return container.NewStack(bg, container.NewCenter(text))
}

func serverRow(snapshot serverSnapshot, darkMode bool) fyne.CanvasObject {
	name := widget.NewLabelWithStyle(snapshot.Server.DisplayName(), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	name.Wrapping = fyne.TextWrapOff
	name.Truncation = fyne.TextTruncateEllipsis

	header := container.NewBorder(nil, nil, nil, connectivityBadge(snapshot.Connected), name)

	updatedLabel := "Updated " + relativeTime(serverUpdatedTime(snapshot))
	if !snapshot.Connected && !snapshot.LastSuccess.IsZero() {
		updatedLabel = "Last ok " + relativeTime(snapshot.LastSuccess)
	}

	objects := []fyne.CanvasObject{header, metaText(updatedLabel, darkMode)}

	if snapshot.Err != nil {
		objects = append(objects, metaText(briefError(snapshot.Err.Error(), 80), darkMode))
	}

	return container.NewVBox(objects...)
}

func settingsServerRows(app *App, snapshots []serverSnapshot, darkMode bool, selectedID string) []fyne.CanvasObject {
	if len(snapshots) == 0 {
		return []fyne.CanvasObject{widget.NewLabel("No servers configured")}
	}

	objects := make([]fyne.CanvasObject, 0, len(snapshots)*2)
	for i, snapshot := range snapshots {
		server := snapshot.Server
		selected := server.ID == selectedID

		editButton := widget.NewButtonWithIcon("Edit (E)", theme.DocumentCreateIcon(), func() {
			app.selectSettingsServer(server.ID)
			serverCopy := server
			app.showServerEditor(&serverCopy)
		})
		deleteButton := widget.NewButtonWithIcon("Delete (X)", theme.DeleteIcon(), func() {
			app.selectSettingsServer(server.ID)
			app.showDeleteServerConfirm(server)
		})

		rowBody := container.NewVBox(
			serverRow(snapshot, darkMode),
			container.NewHBox(editButton, deleteButton),
		)

		border := canvas.NewRectangle(color.Transparent)
		border.CornerRadius = 14
		border.StrokeWidth = 1.5
		border.StrokeColor = cardBorder(darkMode, false)
		if selected {
			border.StrokeColor = sourceBadgeFill(darkMode)
		}

		objects = append(objects, container.NewStack(
			border,
			container.NewPadded(rowBody),
		))
		if i < len(snapshots)-1 {
			objects = append(objects, widget.NewSeparator())
		}
	}
	return objects
}

func (a *App) selectSettingsServer(serverID string) {
	a.stateMu.Lock()
	a.selectedSettingsServerID = strings.TrimSpace(serverID)
	a.stateMu.Unlock()
	a.refreshSettingsDialogContent()
}

func (a *App) selectedSettingsServer() (BuddyServer, bool) {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	for _, server := range a.servers {
		if server.ID == a.selectedSettingsServerID {
			return server, true
		}
	}
	if len(a.servers) > 0 {
		return a.servers[0], true
	}
	return BuddyServer{}, false
}

func (a *App) editSelectedSettingsServer() {
	server, ok := a.selectedSettingsServer()
	if !ok {
		return
	}
	serverCopy := server
	a.showServerEditor(&serverCopy)
}

func (a *App) confirmDeleteSelectedSettingsServer() {
	server, ok := a.selectedSettingsServer()
	if !ok {
		return
	}
	a.showDeleteServerConfirm(server)
}

func (a *App) showDeleteServerConfirm(server BuddyServer) {
	content := widget.NewLabel("Delete " + server.DisplayName() + "?")
	content.Wrapping = fyne.TextWrapWord

	confirm := dialog.NewCustomConfirm("Delete Server", "Delete (Enter)", "Cancel (Esc)", content, func(ok bool) {
		if ok {
			a.deleteServer(server)
		}
	}, a.window)
	confirm.SetOnClosed(func() {
		a.stateMu.Lock()
		if a.settingsDelete == confirm {
			a.settingsDelete = nil
		}
		a.stateMu.Unlock()
	})

	a.stateMu.Lock()
	a.settingsDelete = confirm
	a.stateMu.Unlock()
	confirm.Show()
}

func (a *App) toggleExitNodeMenu() {
	a.stateMu.Lock()
	a.exitNodeMenuOpen = !a.exitNodeMenuOpen
	a.stateMu.Unlock()
	fyne.Do(a.render)
}

func (a *App) setExitNode(node tailscaleExitNode) {
	target := strings.TrimSpace(node.IP)
	if target == "" {
		target = strings.TrimSpace(node.Name)
	}
	go a.updateExitNode(target, "Exit node enabled: "+valueOrDash(node.Name))
}

func (a *App) disableExitNode() {
	go a.updateExitNode("", "Exit node disabled")
}

func (a *App) updateExitNode(ip, successMessage string) {
	a.stateMu.Lock()
	a.tailscaleBusy = true
	a.stateMu.Unlock()
	fyne.Do(a.render)

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	err := setTailscaleExitNode(ctx, ip)

	a.stateMu.Lock()
	a.tailscaleBusy = false
	if err == nil {
		a.exitNodeMenuOpen = false
	}
	a.stateMu.Unlock()

	if err != nil {
		fyne.Do(func() {
			a.render()
			dialog.ShowError(err, a.window)
		})
		return
	}

	a.setStatusLine(successMessage)
	_ = a.refreshNow(context.Background(), true)
}

func (a *App) renderTailscale(state tailscaleState, darkMode bool, menuOpen bool, busy bool) {
	a.exitNodeButton.SetTexts("TS", exitNodeButtonLabel(state))
	a.exitNodeButton.SetFills(tailscaleSegmentFill(state), exitNodeBadgeFill(state))
	if busy || !state.Installed {
		a.exitNodeButton.Disable()
	} else {
		a.exitNodeButton.Enable()
	}
	a.exitNodeButton.Refresh()

	if !menuOpen {
		a.exitNodeMenu.Objects = nil
		a.exitNodeMenu.Refresh()
		return
	}

	bodyItems := []fyne.CanvasObject{
		widget.NewLabelWithStyle("Exit Nodes", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		metaText("Click current node again to disable it.", darkMode),
	}

	switch {
	case state.Error != "":
		bodyItems = append(bodyItems, metaText(briefError(state.Error, 120), darkMode))
	case len(state.ExitNodes) == 0 && state.ExitNodeName == "":
		bodyItems = append(bodyItems, metaText("No exit nodes available", darkMode))
	default:
		if state.ExitNodeName != "" && !tailscaleCurrentNodeListed(state) {
			disableCurrent := widget.NewButton("Current: "+state.ExitNodeName+" (disable)", a.disableExitNode)
			if busy {
				disableCurrent.Disable()
			}
			bodyItems = append(bodyItems, disableCurrent)
		}
		for _, node := range state.ExitNodes {
			node := node
			button := widget.NewButton(exitNodeMenuLabel(node), func() {
				if node.Current {
					a.disableExitNode()
					return
				}
				a.setExitNode(node)
			})
			if busy || (!node.Online && !node.Current) {
				button.Disable()
			}
			if node.Current {
				button.Importance = widget.HighImportance
			}
			bodyItems = append(bodyItems, button)
		}
	}

	fillRect := canvas.NewRectangle(cardFill(darkMode))
	fillRect.CornerRadius = 18

	border := canvas.NewRectangle(color.Transparent)
	border.CornerRadius = 18
	border.StrokeWidth = 1.5
	border.StrokeColor = cardBorder(darkMode, false)

	a.exitNodeMenu.Objects = []fyne.CanvasObject{
		container.NewStack(
			fillRect,
			border,
			container.NewPadded(container.NewVBox(bodyItems...)),
		),
	}
	a.exitNodeMenu.Refresh()
}

func tailscaleSegmentFill(state tailscaleState) color.Color {
	switch {
	case state.Online:
		return color.NRGBA{R: 0x2D, G: 0x7A, B: 0x52, A: 0xFF}
	default:
		return color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF}
	}
}

func exitNodeButtonLabel(state tailscaleState) string {
	if state.Error != "" {
		return "Exit !"
	}
	return "Exit"
}

func exitNodeBadgeFill(state tailscaleState) color.Color {
	switch {
	case !state.Installed:
		return color.NRGBA{R: 0x34, G: 0x35, B: 0x39, A: 0xFF}
	case state.Error != "":
		return color.NRGBA{R: 0x8A, G: 0x61, B: 0x18, A: 0xFF}
	case state.ExitNodeName != "":
		return color.NRGBA{R: 0x2D, G: 0x7A, B: 0x52, A: 0xFF}
	default:
		return color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF}
	}
}

func exitNodeMenuLabel(node tailscaleExitNode) string {
	label := node.Name
	if label == "" {
		label = valueOrDash(node.IP)
	}
	switch {
	case node.Current:
		return label + " (disable)"
	case !node.Online:
		return label + " (offline)"
	default:
		return label
	}
}

func tailscaleCurrentNodeListed(state tailscaleState) bool {
	for _, node := range state.ExitNodes {
		if node.Current {
			return true
		}
	}
	return false
}

func compactExitNodeName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimSuffix(trimmed, ".")
	if parts := strings.Split(trimmed, "."); len(parts) > 0 && parts[0] != "" {
		trimmed = parts[0]
	}
	if len(trimmed) > 14 {
		return trimmed[:14]
	}
	return trimmed
}

func disabledBadgeFill(fill color.Color) color.Color {
	value, ok := color.NRGBAModel.Convert(fill).(color.NRGBA)
	if !ok {
		return fill
	}
	return color.NRGBA{
		R: uint8((uint16(value.R) + 0x7A) / 2),
		G: uint8((uint16(value.G) + 0x7A) / 2),
		B: uint8((uint16(value.B) + 0x7A) / 2),
		A: value.A,
	}
}

func sessionRow(session SessionResponse, darkMode bool) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(sessionListTitle(session), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Wrapping = fyne.TextWrapOff
	title.Truncation = fyne.TextTruncateEllipsis

	badges := container.NewHBox(
		sourceBadge(session.ServerName, darkMode),
		stateBadge(session.State),
	)

	header := container.NewBorder(
		nil,
		nil,
		nil,
		badges,
		title,
	)

	var middle fyne.CanvasObject = layout.NewSpacer()
	if summary := sessionSummaryObject(session, darkMode, ""); summary != nil {
		middle = summary
	}
	return container.NewBorder(
		header,
		metaText("Updated "+relativeTime(session.UpdatedAt), darkMode),
		nil,
		nil,
		middle,
	)
}

func sessionCell(session SessionResponse, darkMode bool) fyne.CanvasObject {
	return container.NewPadded(sessionRow(session, darkMode))
}

func stateBadge(state model.State) fyne.CanvasObject {
	label, fill := sessionBadgeStyle(state)
	bg := canvas.NewRectangle(fill)
	bg.SetMinSize(fyne.NewSize(110, 32))
	bg.CornerRadius = 10

	text := canvas.NewText(strings.ToUpper(label), color.White)
	text.Alignment = fyne.TextAlignCenter
	text.TextStyle = fyne.TextStyle{Bold: true}
	text.TextSize = 22

	return container.NewStack(bg, container.NewCenter(text))
}

func connectivityBadge(connected bool) fyne.CanvasObject {
	label := "offline"
	fill := color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF}
	if connected {
		label = "online"
		fill = color.NRGBA{R: 0x2D, G: 0x7A, B: 0x52, A: 0xFF}
	}

	bg := canvas.NewRectangle(fill)
	bg.SetMinSize(fyne.NewSize(110, 32))
	bg.CornerRadius = 10

	text := canvas.NewText(strings.ToUpper(label), color.White)
	text.Alignment = fyne.TextAlignCenter
	text.TextStyle = fyne.TextStyle{Bold: true}
	text.TextSize = 22

	return container.NewStack(bg, container.NewCenter(text))
}

func serverStatusChip(name string, connected bool, darkMode bool) fyne.CanvasObject {
	label := valueOrDash(name)
	textColor := color.Color(color.White)
	if !connected {
		textColor = offlineServerText(darkMode)
	}

	text := canvas.NewText(label, textColor)
	text.Alignment = fyne.TextAlignCenter
	text.TextStyle = fyne.TextStyle{Bold: true}
	text.TextSize = 22

	paddingWidth := float32(28)
	minWidth := float32(112)
	width := text.MinSize().Width + paddingWidth
	if width < minWidth {
		width = minWidth
	}

	bg := canvas.NewRectangle(serverStatusFill(connected))
	bg.SetMinSize(fyne.NewSize(width, headerControlHeight))
	bg.CornerRadius = 10

	return container.NewStack(bg, container.NewCenter(text))
}

func sourceBadge(name string, darkMode bool) fyne.CanvasObject {
	label := valueOrDash(name)

	text := canvas.NewText(label, color.White)
	text.Alignment = fyne.TextAlignCenter
	text.TextStyle = fyne.TextStyle{Bold: true}
	text.TextSize = 22

	paddingWidth := float32(28)
	minWidth := float32(96)
	width := text.MinSize().Width + paddingWidth
	if width < minWidth {
		width = minWidth
	}

	bg := canvas.NewRectangle(sourceBadgeFill(darkMode))
	bg.SetMinSize(fyne.NewSize(width, 32))
	bg.CornerRadius = 10

	return container.NewStack(bg, container.NewCenter(text))
}

func metaText(value string, darkMode bool) fyne.CanvasObject {
	text := canvas.NewText(value, metaForeground(darkMode))
	text.TextSize = 22
	text.Alignment = fyne.TextAlignLeading
	return text
}

func markdownText(value string, darkMode bool) fyne.CanvasObject {
	rich := widget.NewRichTextFromMarkdown(spacedMarkdown(value))
	objects := spacedMarkdownObjects(markdownObjects(rich.Segments, darkMode))
	if len(objects) == 0 {
		return layout.NewSpacer()
	}
	if len(objects) == 1 {
		return objects[0]
	}
	return container.NewVBox(objects...)
}

func sessionSummaryObject(session SessionResponse, darkMode bool, fallback string) fyne.CanvasObject {
	if htmlValue := firstNonEmptyText(session.OpenSummaryHTML, session.SummaryHTML); htmlValue != "" {
		if objects := spacedMarkdownObjects(htmlSummaryObjects(htmlValue, darkMode)); len(objects) > 0 {
			if len(objects) == 1 {
				return objects[0]
			}
			return container.NewVBox(objects...)
		}
	}

	markdownValue := firstNonEmptyText(
		session.OpenSummaryMarkdown,
		session.OpenSummary,
		session.SummaryMarkdown,
		session.Summary,
		fallback,
	)
	if markdownValue == "" {
		return nil
	}
	return markdownText(markdownValue, darkMode)
}

func spacedMarkdownObjects(objects []fyne.CanvasObject) []fyne.CanvasObject {
	if len(objects) < 2 {
		return objects
	}

	spaced := make([]fyne.CanvasObject, 0, len(objects)*2-1)
	for i, object := range objects {
		if i > 0 {
			gap := canvas.NewRectangle(color.Transparent)
			gap.SetMinSize(fyne.NewSize(1, markdownLineHeight(widget.RichTextStyleInline)))
			spaced = append(spaced, gap)
		}
		spaced = append(spaced, object)
	}
	return spaced
}

func htmlSummaryObjects(value string, darkMode bool) []fyne.CanvasObject {
	document, err := xhtml.Parse(strings.NewReader("<!doctype html><html><body>" + value + "</body></html>"))
	if err != nil {
		return nil
	}
	body := findHTMLBodyNode(document)
	if body == nil {
		return nil
	}

	objects := make([]fyne.CanvasObject, 0, 8)
	for node := body.FirstChild; node != nil; node = node.NextSibling {
		objects = append(objects, htmlSummaryNodeObjects(node, darkMode)...)
	}
	return objects
}

func findHTMLBodyNode(node *xhtml.Node) *xhtml.Node {
	if node == nil {
		return nil
	}
	if node.Type == xhtml.ElementNode && node.Data == "body" {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLBodyNode(child); found != nil {
			return found
		}
	}
	return nil
}

func htmlSummaryNodeObjects(node *xhtml.Node, darkMode bool) []fyne.CanvasObject {
	switch node.Type {
	case xhtml.TextNode:
		text := normalizeHTMLText(node.Data)
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []fyne.CanvasObject{newMarkdownParagraph([]markdownSpan{{
			Text:  text,
			Style: widget.RichTextStyleInline,
		}})}
	case xhtml.ElementNode:
		switch node.Data {
		case "p":
			return htmlParagraphObjects(node, darkMode, "")
		case "pre":
			text := strings.Trim(htmlNodePlainText(node), "\n")
			if text == "" {
				return nil
			}
			return []fyne.CanvasObject{newMarkdownCodeBlockObject(text, darkMode)}
		case "ul":
			return htmlListObjects(node, false, darkMode)
		case "ol":
			return htmlListObjects(node, true, darkMode)
		case "hr":
			return []fyne.CanvasObject{widget.NewSeparator()}
		default:
			objects := make([]fyne.CanvasObject, 0, 4)
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				objects = append(objects, htmlSummaryNodeObjects(child, darkMode)...)
			}
			return objects
		}
	default:
		return nil
	}
}

func htmlListObjects(node *xhtml.Node, ordered bool, darkMode bool) []fyne.CanvasObject {
	objects := make([]fyne.CanvasObject, 0, 4)
	index := 1
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != xhtml.ElementNode || child.Data != "li" {
			continue
		}

		prefix := "\u2022 "
		if ordered {
			prefix = fmt.Sprintf("%d. ", index)
		}
		objects = append(objects, htmlParagraphObjects(child, darkMode, prefix)...)
		index++
	}
	return objects
}

func htmlParagraphObjects(node *xhtml.Node, darkMode bool, prefix string) []fyne.CanvasObject {
	lines := htmlParagraphLines(node, prefix)
	if len(lines) == 0 {
		return nil
	}

	if len(lines) > 1 {
		switch {
		case markdownLinesAllCodeOnly(lines):
			return []fyne.CanvasObject{newMarkdownCodeBlockObject(markdownCodeLinesText(lines), darkMode)}
		case !markdownLineIsCodeOnly(lines[0]) && markdownLinesAllCodeOnly(lines[1:]):
			objects := []fyne.CanvasObject{newMarkdownParagraph(markdownJoinLines(lines[:1]))}
			objects = append(objects, newMarkdownCodeBlockObject(markdownCodeLinesText(lines[1:]), darkMode))
			return objects
		}
	}

	return []fyne.CanvasObject{newMarkdownParagraph(markdownJoinLines(lines))}
}

func htmlParagraphLines(node *xhtml.Node, prefix string) [][]markdownSpan {
	lines := [][]markdownSpan{{}}
	if prefix != "" {
		lines[0] = append(lines[0], markdownSpan{Text: prefix, Style: widget.RichTextStyleInline})
	}
	htmlAppendInlineNode(&lines, node, widget.RichTextStyleInline)
	return normalizeMarkdownLines(lines)
}

func htmlAppendInlineNode(lines *[][]markdownSpan, node *xhtml.Node, style widget.RichTextStyle) {
	switch node.Type {
	case xhtml.TextNode:
		text := normalizeHTMLText(node.Data)
		if text == "" {
			return
		}
		htmlAppendLineSpan(lines, markdownSpan{Text: text, Style: style})
	case xhtml.ElementNode:
		switch node.Data {
		case "br":
			*lines = append(*lines, nil)
			return
		case "code":
			htmlAppendLineSpan(lines, markdownSpan{
				Text:  htmlNodePlainText(node),
				Style: mergedMarkdownStyle(style, markdownCodeInlineStyle()),
			})
			return
		case "a":
			linkStyle := mergedMarkdownStyle(style, markdownLinkStyle())
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				htmlAppendInlineNode(lines, child, linkStyle)
			}
			return
		case "strong", "b":
			style = mergedMarkdownStyle(style, widget.RichTextStyle{
				Inline:    true,
				SizeName:  theme.SizeNameText,
				TextStyle: fyne.TextStyle{Bold: true},
			})
		case "em", "i":
			style = mergedMarkdownStyle(style, widget.RichTextStyle{
				Inline:    true,
				SizeName:  theme.SizeNameText,
				TextStyle: fyne.TextStyle{Italic: true},
			})
		case "p", "li", "span":
		default:
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		htmlAppendInlineNode(lines, child, style)
	}
}

func htmlAppendLineSpan(lines *[][]markdownSpan, span markdownSpan) {
	if span.Text == "" {
		return
	}
	if len(*lines) == 0 {
		*lines = append(*lines, nil)
	}
	lastIndex := len(*lines) - 1
	(*lines)[lastIndex] = append((*lines)[lastIndex], span)
}

func normalizeHTMLText(value string) string {
	value = stdhtml.UnescapeString(value)
	if value == "" {
		return ""
	}

	var builder strings.Builder
	lastSpace := false
	for _, r := range value {
		if unicode.IsSpace(r) {
			if lastSpace {
				continue
			}
			builder.WriteByte(' ')
			lastSpace = true
			continue
		}
		builder.WriteRune(r)
		lastSpace = false
	}
	return builder.String()
}

func htmlNodePlainText(node *xhtml.Node) string {
	var builder strings.Builder
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		switch current.Type {
		case xhtml.TextNode:
			builder.WriteString(stdhtml.UnescapeString(current.Data))
		case xhtml.ElementNode:
			if current.Data == "br" {
				builder.WriteByte('\n')
				return
			}
			for child := current.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walk(child)
	}
	return builder.String()
}

func mergedMarkdownStyle(base, overlay widget.RichTextStyle) widget.RichTextStyle {
	style := base
	if style.SizeName == "" {
		style.SizeName = theme.SizeNameText
	}
	if overlay.ColorName != "" {
		style.ColorName = overlay.ColorName
	}
	if overlay.SizeName != "" {
		style.SizeName = overlay.SizeName
	}
	style.Inline = style.Inline || overlay.Inline
	style.TextStyle.Bold = style.TextStyle.Bold || overlay.TextStyle.Bold
	style.TextStyle.Italic = style.TextStyle.Italic || overlay.TextStyle.Italic
	style.TextStyle.Monospace = style.TextStyle.Monospace || overlay.TextStyle.Monospace
	return style
}

func normalizeMarkdownLines(lines [][]markdownSpan) [][]markdownSpan {
	normalized := make([][]markdownSpan, 0, len(lines))
	for _, line := range lines {
		compacted := compactMarkdownSpans(compactInlineBoundarySpaces(trimMarkdownLine(line)))
		if len(compacted) == 0 {
			normalized = append(normalized, nil)
			continue
		}
		normalized = append(normalized, compacted)
	}

	for len(normalized) > 0 && len(normalized[0]) == 0 {
		normalized = normalized[1:]
	}
	for len(normalized) > 0 && len(normalized[len(normalized)-1]) == 0 {
		normalized = normalized[:len(normalized)-1]
	}
	return normalized
}

func trimMarkdownLine(line []markdownSpan) []markdownSpan {
	if len(line) == 0 {
		return nil
	}

	trimmed := append([]markdownSpan(nil), line...)
	for len(trimmed) > 0 {
		updated := strings.TrimLeftFunc(trimmed[0].Text, unicode.IsSpace)
		if updated == "" {
			trimmed = trimmed[1:]
			continue
		}
		trimmed[0].Text = updated
		break
	}
	for len(trimmed) > 0 {
		lastIndex := len(trimmed) - 1
		updated := strings.TrimRightFunc(trimmed[lastIndex].Text, unicode.IsSpace)
		if updated == "" {
			trimmed = trimmed[:lastIndex]
			continue
		}
		trimmed[lastIndex].Text = updated
		break
	}
	return trimmed
}

func compactInlineBoundarySpaces(line []markdownSpan) []markdownSpan {
	if len(line) < 2 {
		return line
	}

	compacted := append([]markdownSpan(nil), line...)
	for i := 0; i < len(compacted)-1; i++ {
		left := compacted[i]
		right := compacted[i+1]

		leftRune, leftOK := lastNonSpaceRune(left.Text)
		rightRune, rightOK := firstNonSpaceRune(right.Text)
		if !leftOK || !rightOK {
			continue
		}

		if hasStyledBoundary(left, right) && isCJKKanaHangulRune(leftRune) && isCJKKanaHangulRune(rightRune) {
			left.Text = strings.TrimRightFunc(left.Text, unicode.IsSpace)
			right.Text = strings.TrimLeftFunc(right.Text, unicode.IsSpace)
		}

		if hasStyledBoundary(left, right) && isCJKKanaHangulRune(leftRune) {
			right.Text = strings.TrimLeftFunc(right.Text, unicode.IsSpace)
		}
		if hasStyledBoundary(left, right) && isCJKKanaHangulRune(rightRune) {
			left.Text = strings.TrimRightFunc(left.Text, unicode.IsSpace)
		}

		compacted[i] = left
		compacted[i+1] = right
	}

	return compactMarkdownSpans(compacted)
}

func hasStyledBoundary(left, right markdownSpan) bool {
	return isInlineAccentSpan(left) || isInlineAccentSpan(right)
}

func isInlineAccentSpan(span markdownSpan) bool {
	return isInlineAccentStyle(span.Style)
}

func isInlineAccentStyle(style widget.RichTextStyle) bool {
	return style.TextStyle.Monospace || style.ColorName == theme.ColorNamePrimary || style.TextStyle.Italic
}

func firstNonSpaceRune(value string) (rune, bool) {
	for _, r := range value {
		if !unicode.IsSpace(r) {
			return r, true
		}
	}
	return 0, false
}

func lastNonSpaceRune(value string) (rune, bool) {
	last := rune(0)
	found := false
	for _, r := range value {
		if unicode.IsSpace(r) {
			continue
		}
		last = r
		found = true
	}
	return last, found
}

func isCJKKanaHangulRune(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}

func markdownJoinLines(lines [][]markdownSpan) []markdownSpan {
	joined := make([]markdownSpan, 0, len(lines)*2)
	for i, line := range lines {
		if i > 0 {
			joined = append(joined, markdownSpan{Text: "\n", Style: widget.RichTextStyleInline})
		}
		joined = append(joined, line...)
	}
	return compactMarkdownSpans(joined)
}

func markdownLinesAllCodeOnly(lines [][]markdownSpan) bool {
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if !markdownLineIsCodeOnly(line) {
			return false
		}
	}
	return true
}

func markdownLineIsCodeOnly(line []markdownSpan) bool {
	hasCode := false
	for _, span := range line {
		if strings.TrimSpace(span.Text) == "" {
			continue
		}
		if !span.Style.TextStyle.Monospace {
			return false
		}
		hasCode = true
	}
	return hasCode
}

func markdownCodeLinesText(lines [][]markdownSpan) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		var builder strings.Builder
		for _, span := range line {
			builder.WriteString(span.Text)
		}
		text := strings.TrimSpace(builder.String())
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func markdownObjects(segments []widget.RichTextSegment, darkMode bool) []fyne.CanvasObject {
	objects := make([]fyne.CanvasObject, 0, len(segments))
	for _, segment := range segments {
		switch item := segment.(type) {
		case *widget.TextSegment:
			if item.Style == widget.RichTextStyleCodeBlock {
				objects = append(objects, newMarkdownCodeBlockObject(item.Text, darkMode))
				continue
			}
			objects = append(objects, newMarkdownParagraph([]markdownSpan{markdownSpanFromTextSegment(item)}))
		case *widget.ParagraphSegment:
			if text, ok := markdownCodeOnlyText(item.Texts); ok {
				objects = append(objects, newMarkdownCodeBlockObject(text, darkMode))
				continue
			}
			if spans := markdownSpans(item.Texts); len(spans) > 0 {
				objects = append(objects, newMarkdownParagraph(spans))
			}
		case *widget.ListSegment:
			objects = append(objects, markdownObjects(item.Segments(), darkMode)...)
		case *widget.HyperlinkSegment:
			objects = append(objects, newMarkdownParagraph([]markdownSpan{{
				Text:  item.Text,
				Style: markdownLinkStyle(),
			}}))
		case *widget.SeparatorSegment:
			objects = append(objects, widget.NewSeparator())
		}
	}
	return objects
}

func markdownCodeOnlyText(segments []widget.RichTextSegment) (string, bool) {
	var builder strings.Builder
	hasCode := false

	for _, segment := range segments {
		switch item := segment.(type) {
		case *widget.TextSegment:
			switch item.Style {
			case widget.RichTextStyleCodeInline:
				builder.WriteString(item.Text)
				hasCode = true
			case widget.RichTextStyleInline:
				if strings.TrimSpace(item.Text) != "" {
					return "", false
				}
				builder.WriteString(item.Text)
			default:
				return "", false
			}
		default:
			return "", false
		}
	}

	if !hasCode {
		return "", false
	}
	return strings.Trim(builder.String(), "\n"), true
}

func markdownSpans(segments []widget.RichTextSegment) []markdownSpan {
	spans := make([]markdownSpan, 0, len(segments))
	for _, segment := range segments {
		switch item := segment.(type) {
		case *widget.TextSegment:
			if item.Style == widget.RichTextStyleCodeBlock {
				continue
			}
			spans = append(spans, markdownSpanFromTextSegment(item))
		case *widget.HyperlinkSegment:
			spans = append(spans, markdownSpan{
				Text:  item.Text,
				Style: markdownLinkStyle(),
			})
		case *widget.ParagraphSegment:
			spans = append(spans, markdownSpans(item.Texts)...)
		}
	}
	return compactMarkdownSpans(spans)
}

func markdownSpanFromTextSegment(segment *widget.TextSegment) markdownSpan {
	style := segment.Style
	switch segment.Style {
	case widget.RichTextStyleCodeInline:
		style = markdownCodeInlineStyle()
	case widget.RichTextStyleInline:
		style = widget.RichTextStyleInline
	}
	return markdownSpan{
		Text:  segment.Text,
		Style: style,
	}
}

func markdownCodeInlineStyle() widget.RichTextStyle {
	return widget.RichTextStyle{
		ColorName: theme.ColorNamePrimary,
		Inline:    true,
		SizeName:  theme.SizeNameText,
		TextStyle: fyne.TextStyle{Monospace: true, Bold: true},
	}
}

func markdownLinkStyle() widget.RichTextStyle {
	return widget.RichTextStyle{
		ColorName: theme.ColorNamePrimary,
		Inline:    true,
		SizeName:  theme.SizeNameText,
		TextStyle: fyne.TextStyle{Bold: true},
	}
}

func spacedMarkdown(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return value
}

type markdownCodeBlockSegment struct {
	Text     string
	DarkMode bool
}

func (s *markdownCodeBlockSegment) Inline() bool {
	return false
}

func (s *markdownCodeBlockSegment) Textual() string {
	return s.Text
}

func (s *markdownCodeBlockSegment) Visual() fyne.CanvasObject {
	return newMarkdownCodeBlockObject(s.Text, s.DarkMode)
}

func (s *markdownCodeBlockSegment) Update(o fyne.CanvasObject) {
	containerObj, ok := o.(*fyne.Container)
	if !ok {
		return
	}
	replacement, ok := newMarkdownCodeBlockObject(s.Text, s.DarkMode).(*fyne.Container)
	if !ok {
		return
	}
	containerObj.Layout = replacement.Layout
	containerObj.Objects = replacement.Objects
	containerObj.Refresh()
}

func (s *markdownCodeBlockSegment) Select(_, _ fyne.Position) {}

func (s *markdownCodeBlockSegment) SelectedText() string {
	return ""
}

func (s *markdownCodeBlockSegment) Unselect() {}

func newMarkdownCodeBlockObject(text string, darkMode bool) fyne.CanvasObject {
	_ = darkMode

	code := widget.NewRichText(&widget.TextSegment{
		Style: widget.RichTextStyle{
			Inline:    false,
			SizeName:  theme.SizeNameText,
			TextStyle: fyne.TextStyle{Monospace: true, Bold: true},
		},
		Text: text,
	})
	code.Wrapping = fyne.TextWrapBreak
	return code
}

func metaForeground(darkMode bool) color.Color {
	if darkMode {
		return color.NRGBA{R: 0xB9, G: 0xC0, B: 0xCB, A: 0xFF}
	}
	return color.NRGBA{R: 0x71, G: 0x67, B: 0x5A, A: 0xFF}
}

func sourceBadgeFill(darkMode bool) color.NRGBA {
	if darkMode {
		return color.NRGBA{R: 0x5A, G: 0x45, B: 0x2A, A: 0xFF}
	}
	return color.NRGBA{R: 0xA3, G: 0x73, B: 0x2D, A: 0xFF}
}

func serverStatusFill(connected bool) color.NRGBA {
	if connected {
		return color.NRGBA{R: 0x2D, G: 0x7A, B: 0x52, A: 0xFF}
	}
	return color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF}
}

func offlineServerText(darkMode bool) color.Color {
	if darkMode {
		return color.NRGBA{R: 0xCF, G: 0xD4, B: 0xDB, A: 0xFF}
	}
	return color.NRGBA{R: 0xE5, G: 0xE7, B: 0xEB, A: 0xFF}
}

func serverUpdatedTime(snapshot serverSnapshot) time.Time {
	if !snapshot.Status.ServerTime.IsZero() {
		return snapshot.Status.ServerTime
	}
	if !snapshot.LastSuccess.IsZero() {
		return snapshot.LastSuccess
	}
	return snapshot.FetchedAt
}

func sessionActionKey(session SessionResponse) string {
	return strings.TrimSpace(session.ServerID) + ":" + strings.TrimSpace(session.SessionID)
}

func findSessionByActionKey(sessions []SessionResponse, actionKey string) *SessionResponse {
	for i := range sessions {
		if sessionActionKey(sessions[i]) == actionKey {
			return &sessions[i]
		}
	}
	return nil
}

func isOpenSession(session SessionResponse) bool {
	return session.State == model.StateAttention
}

func canContinueSession(session SessionResponse) bool {
	return session.CanContinue && session.ContinueAction != nil
}

func cloneShortcutMap(source map[string]fyne.KeyName) map[string]fyne.KeyName {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]fyne.KeyName, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func clonePendingMap(source map[string]bool) map[string]bool {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]bool, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func sessionLabelByID(sessions []SessionResponse, serverID, sessionID string) string {
	if session := findSession(sessions, serverID, sessionID); session != nil {
		return sessionLabel(*session)
	}
	return valueOrDash(sessionID)
}

func findSession(sessions []SessionResponse, serverID, sessionID string) *SessionResponse {
	for i := range sessions {
		if sessions[i].SessionID != sessionID {
			continue
		}
		if strings.TrimSpace(serverID) != "" && sessions[i].ServerID != serverID {
			continue
		}
		return &sessions[i]
	}
	return nil
}

func sessionLabel(session SessionResponse) string {
	if strings.TrimSpace(session.DisplayTitle) != "" {
		return session.DisplayTitle
	}
	if strings.TrimSpace(session.ShortSessionID) != "" {
		return session.ShortSessionID
	}
	return valueOrDash(session.SessionID)
}

func sessionListTitle(session SessionResponse) string {
	label := sessionLabel(session)
	if label == "-" {
		return "unknown"
	}
	return label
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func relativeTime(when time.Time) string {
	if when.IsZero() {
		return "-"
	}
	delta := time.Since(when)
	if delta < 0 {
		delta = 0
	}
	if delta < time.Minute {
		return "just now"
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm ago", int(delta/time.Minute))
	}
	if delta < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(delta/time.Hour))
	}
	return when.Local().Format("01-02 15:04")
}

func canContinue(item NotificationResponse) bool {
	for _, action := range item.Actions {
		if action == model.NotificationActionContinue {
			return true
		}
	}
	return false
}

func shouldPopupNotification(item NotificationResponse) bool {
	return item.Kind == model.NotificationError && item.State == model.NotificationPending
}

func briefError(message string, limit int) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if limit <= 0 || len(message) <= limit {
		return message
	}
	if limit <= 3 {
		return message[:limit]
	}
	return message[:limit-3] + "..."
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func (a *App) toggleTheme() {
	a.darkMode = !a.darkMode
	a.applyTheme()
	fyne.Do(func() {
		a.render()
		a.refreshSettingsDialogContent()
	})
}

func (a *App) applyTheme() {
	if a.fyneApp == nil {
		return
	}
	base := fyne.Theme(theme.DefaultTheme())
	var variant fyne.ThemeVariant
	if a.darkMode {
		variant = theme.VariantDark
	} else {
		variant = theme.VariantLight
	}
	base = forcedVariant{Theme: base, variant: variant}
	a.fyneApp.Settings().SetTheme(textSizeTheme{Theme: base})
}

func (a *App) effectiveDarkMode() bool {
	return a.darkMode
}

func (a *App) dismissNotification() {
	a.stateMu.RLock()
	currentDialog := a.notifDialog
	a.stateMu.RUnlock()
	if currentDialog != nil {
		currentDialog.Hide()
	}
}

func (a *App) showHelpDialog() {
	a.stateMu.RLock()
	currentDialog := a.helpDialog
	a.stateMu.RUnlock()
	if currentDialog != nil {
		currentDialog.Show()
		return
	}

	content := widget.NewLabel(strings.Join([]string{
		"Press ? anytime to open this window.",
		"",
		"?  Show this help",
		"R  Refresh all servers",
		"S  Server settings",
		"T  Toggle theme",
		"J/K  Scroll",
		"A  Acknowledge notification",
		"C  Continue current session",
		"Hold open-session letter 1s  Approve session",
		"Esc  Close popup/dialog",
	}, "\n"))
	content.Wrapping = fyne.TextWrapWord

	dlg := dialog.NewCustom("Shortcuts", "Close (Esc)", content, a.window)
	dlg.Resize(fyne.NewSize(760, 420))
	dlg.SetOnClosed(func() {
		a.stateMu.Lock()
		if a.helpDialog == dlg {
			a.helpDialog = nil
		}
		a.stateMu.Unlock()
	})

	a.stateMu.Lock()
	a.helpDialog = dlg
	a.stateMu.Unlock()
	dlg.Show()
}

func (a *App) scrollBy(delta float32) {
	if a.rootScroll == nil {
		return
	}
	next := a.rootScroll.Offset.Y + delta
	if next < 0 {
		next = 0
	}
	a.rootScroll.Offset.Y = next
	a.rootScroll.Refresh()
}

func (a *App) startScrollHold(key fyne.KeyName, delta float32) {
	a.scrollHoldMu.Lock()
	if a.scrollHoldKey == key {
		a.scrollHoldMu.Unlock()
		return
	}
	if stop := a.scrollHoldStop; stop != nil {
		close(stop)
	}
	stop := make(chan struct{})
	a.scrollHoldStop = stop
	a.scrollHoldKey = key
	a.scrollHoldMu.Unlock()

	go func(stop <-chan struct{}, delta float32, key fyne.KeyName) {
		initialDelay := time.NewTimer(180 * time.Millisecond)
		defer initialDelay.Stop()

		select {
		case <-stop:
			return
		case <-initialDelay.C:
		}

		ticker := time.NewTicker(45 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				fyne.Do(func() {
					a.scrollHoldMu.Lock()
					active := a.scrollHoldKey == key
					a.scrollHoldMu.Unlock()
					if active {
						a.scrollBy(delta)
					}
				})
			}
		}
	}(stop, delta, key)
}

func (a *App) stopScrollHold(key fyne.KeyName) {
	a.scrollHoldMu.Lock()
	defer a.scrollHoldMu.Unlock()
	if a.scrollHoldKey != key {
		return
	}
	if a.scrollHoldStop != nil {
		close(a.scrollHoldStop)
	}
	a.scrollHoldStop = nil
	a.scrollHoldKey = ""
}

func normalizedWindowSize(width, height int) (int, int) {
	if width <= 0 {
		width = 1440
	}
	if height <= 0 {
		height = 900
	}
	if width > 1600 {
		width = 1600
	}
	if height > 1000 {
		height = 1000
	}
	if width < 960 {
		width = 960
	}
	if height < 640 {
		height = 640
	}
	return width, height
}

func themeToggleLabel(darkMode bool) string {
	if darkMode {
		return "Light (T)"
	}
	return "Dark (T)"
}

func appBackground(darkMode bool) color.NRGBA {
	if darkMode {
		return color.NRGBA{R: 0x1B, G: 0x1E, B: 0x23, A: 0xFF}
	}
	return color.NRGBA{R: 0xF6, G: 0xF3, B: 0xEC, A: 0xFF}
}

func cardFill(darkMode bool) color.NRGBA {
	if darkMode {
		return color.NRGBA{R: 0x28, G: 0x2D, B: 0x34, A: 0xFF}
	}
	return color.NRGBA{R: 0xFF, G: 0xFD, B: 0xF8, A: 0xFF}
}

func cardBorder(darkMode bool, highlight bool) color.NRGBA {
	if highlight {
		if darkMode {
			return color.NRGBA{R: 0xD6, G: 0x8A, B: 0x24, A: 0xFF}
		}
		return color.NRGBA{R: 0xC9, G: 0x74, B: 0x11, A: 0xFF}
	}
	if darkMode {
		return color.NRGBA{R: 0x6A, G: 0x73, B: 0x80, A: 0xFF}
	}
	return color.NRGBA{R: 0xD2, G: 0xC7, B: 0xB5, A: 0xFF}
}

func aggregateNeedsAttention(state model.State) bool {
	state = normalizeCompatState(state)
	return state == model.StateAttention || state == model.StateError
}
