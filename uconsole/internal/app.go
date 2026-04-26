//go:build uconsole_gui

package uconsole

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	desktop "fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

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

	refreshMu          sync.Mutex
	refreshing         bool
	bgFill             *canvas.Rectangle
	cardFills          []*canvas.Rectangle
	cardBorders        []*canvas.Rectangle
	sessionCardBorder  *canvas.Rectangle
	badgeFill          *canvas.Rectangle
	badgeLabel         *canvas.Text
	updatedLabel       *widget.Label
	serverStrip        *fyne.Container
	sessionList        *fyne.Container
	themeButton        *widget.Button
	refreshButton      *widget.Button
	settingsButton     *widget.Button
	refreshActivity    *widget.Activity
	rootScroll         *container.Scroll
	settingsSummary    *widget.Label
	settingsList       *fyne.Container
	tailscaleState     tailscaleState
	exitNodeButton     *splitBadgeButton
	exitNodeMenu       *fyne.Container
	exitNodeMenuOpen   bool
	tailscaleBusy      bool
	scrollHoldMu       sync.Mutex
	scrollHoldStop     chan struct{}
	scrollHoldKey      fyne.KeyName
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

	LeftText   string
	RightText  string
	LeftFill   color.Color
	RightFill  color.Color
	TextColor  color.Color
	TextSize   float32
	MinWidth   float32
	Disabled   bool
	OnTapped   func()
}

type splitBadgeButtonRenderer struct {
	button       *splitBadgeButton
	leftCap      *canvas.Rectangle
	leftBody     *canvas.Rectangle
	rightBody    *canvas.Rectangle
	rightCap     *canvas.Rectangle
	leftLabel    *canvas.Text
	rightLabel   *canvas.Text
	separator    *canvas.Line
	objects      []fyne.CanvasObject
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
	return fyne.NewSize(width, 38)
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
	return fyne.NewSize(width, 38)
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
		cfg:             cfg,
		logger:          logger,
		clients:         make(map[string]*Client),
		serverSnapshots: make(map[string]serverSnapshot),
		lastStatus:      offlineStatus(),
		statusLine:      "Connecting to codex-buddy",
		darkMode:        true,
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

	a.badgeFill = canvas.NewRectangle(color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF})
	a.badgeFill.SetMinSize(fyne.NewSize(148, 38))
	a.badgeFill.CornerRadius = 12

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

	a.exitNodeButton = newSplitBadgeButton("TS", "Exit", 132, a.toggleExitNodeMenu)
	a.exitNodeMenu = container.NewVBox()

	a.serverStrip = container.NewHBox(widget.NewLabel("No servers configured"))
	serverStripScroll := container.NewHScroll(a.serverStrip)
	serverStripScroll.SetMinSize(fyne.NewSize(260, 38))

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

	a.sessionList = container.NewVBox(widget.NewLabel("No active sessions"))

	content := container.NewVBox(
		a.sectionCard("Sessions", darkMode, a.sessionList, true),
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

func (a *App) sectionCard(title string, darkMode bool, content fyne.CanvasObject, highlightOnAttention bool) fyne.CanvasObject {
	fill := canvas.NewRectangle(cardFill(darkMode))
	fill.CornerRadius = 18

	border := canvas.NewRectangle(color.Transparent)
	border.CornerRadius = 18
	border.StrokeWidth = 1.5
	border.StrokeColor = cardBorder(darkMode, false)

	a.cardFills = append(a.cardFills, fill)
	a.cardBorders = append(a.cardBorders, border)
	if highlightOnAttention {
		a.sessionCardBorder = border
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
		deskCanvas.SetOnKeyUp(func(event *fyne.KeyEvent) {
			switch event.Name {
			case fyne.KeyJ, fyne.KeyK:
				a.stopScrollHold(event.Name)
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
		item.Kind = normalizeCompatNotificationKind(item.Kind)
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
	if len(sessions) > 0 {
		status.ActiveSessionID = sessions[0].SessionID
		status.ActiveSessionDisplayTitle = sessionLabel(sessions[0])
	}
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
	if a.sessionCardBorder != nil {
		a.sessionCardBorder.StrokeColor = cardBorder(darkMode, aggregateNeedsAttention(status.OverallState))
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
	a.renderSessionList(status.Sessions, darkMode)
}

func (a *App) renderServerStrip(snapshots []serverSnapshot, darkMode bool) {
	if a.serverStrip == nil {
		return
	}
	a.serverStrip.Objects = serverStripObjects(snapshots, darkMode)
	a.serverStrip.Refresh()
}

func (a *App) renderSessionList(sessions []SessionResponse, darkMode bool) {
	a.sessionList.Objects = sessionListObjects(sessions, darkMode)
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

func sessionListObjects(sessions []SessionResponse, darkMode bool) []fyne.CanvasObject {
	if len(sessions) == 0 {
		return []fyne.CanvasObject{widget.NewLabel("No active sessions")}
	}

	rows := make([]fyne.CanvasObject, 0, len(sessions))
	for i := 0; i < len(sessions); i += 2 {
		left := sessionCell(sessions[i], darkMode)
		right := fyne.CanvasObject(layout.NewSpacer())
		if i+1 < len(sessions) {
			right = sessionCell(sessions[i+1], darkMode)
		}

		rows = append(rows, container.NewGridWithColumns(2, left, right))
		if i+2 < len(sessions) {
			rows = append(rows, widget.NewSeparator())
		}
	}
	return rows
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
	if summary := firstNonEmptyText(session.AttentionSummary, session.Summary); summary != "" {
		summaryLabel := widget.NewLabel(summary)
		summaryLabel.Wrapping = fyne.TextWrapWord
		middle = summaryLabel
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
	bg.SetMinSize(fyne.NewSize(width, 38))
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
	return item.State == model.NotificationPending
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
