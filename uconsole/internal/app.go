//go:build uconsole_gui

package uconsole

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/vxider/codex-buddy/internal/config"
	"github.com/vxider/codex-buddy/internal/model"
	"github.com/vxider/codex-buddy/uconsole/internal/light"
)

type App struct {
	cfg          config.UConsoleConfig
	logger       *log.Logger
	client       *Client
	fyneApp      fyne.App
	window       fyne.Window
	lightRunner  *light.Runner
	stateMu      sync.RWMutex
	lastStatus   StatusResponse
	lastNotifs   []NotificationResponse
	connected    bool
	statusLine   string
	lastError    string
	lastSuccess  time.Time
	shownNotifID string
	dialogNotifID string
	notifDialog   dialog.Dialog
	statusUntil   time.Time
	darkMode      bool

	bgFill          *canvas.Rectangle
	badgeFill       *canvas.Rectangle
	badgeLabel      *canvas.Text
	titleLabel      *widget.Label
	summaryLabel    *widget.Label
	activeLabel     *widget.Label
	updatedLabel    *widget.Label
	connectionLabel *widget.Label
	statusLabel     *widget.Label
	sessionList     *fyne.Container
	themeButton     *widget.Button
	refreshButton   *widget.Button
	rootScroll      *container.Scroll
}

func Run(ctx context.Context, cfg config.UConsoleConfig, logger *log.Logger) error {
	client := NewClient(cfg.ServerURL, time.Duration(cfg.HTTPTimeoutMS)*time.Millisecond, logger)
	gui := &App{
		cfg:        cfg,
		logger:     logger,
		client:     client,
		lastStatus: offlineStatus(),
		statusLine: "Connecting to codex-buddy",
	}

	gui.fyneApp = app.New()
	gui.applyTheme()
	gui.window = gui.fyneApp.NewWindow("codex-buddy uConsole")
	gui.window.SetMaster()
	if cfg.Window.Fullscreen {
		gui.window.SetFullScreen(true)
	} else {
		width, height := normalizedWindowSize(cfg.Window.Width, cfg.Window.Height)
		gui.window.Resize(fyne.NewSize(float32(width), float32(height)))
	}
	gui.window.SetContent(gui.buildUI())

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
	a.bgFill = canvas.NewRectangle(color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})

	a.badgeFill = canvas.NewRectangle(color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF})
	a.badgeFill.SetMinSize(fyne.NewSize(140, 36))
	a.badgeLabel = canvas.NewText("OFFLINE", color.White)
	a.badgeLabel.Alignment = fyne.TextAlignCenter
	a.badgeLabel.TextStyle = fyne.TextStyle{Bold: true}
	a.badgeLabel.TextSize = 13

	a.titleLabel = widget.NewLabelWithStyle("Codex companion", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	a.titleLabel.Wrapping = fyne.TextWrapWord
	a.summaryLabel = widget.NewLabel("Waiting for the codex-buddy service.")
	a.summaryLabel.Wrapping = fyne.TextWrapWord

	a.activeLabel = widget.NewLabelWithStyle("-", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	a.activeLabel.Wrapping = fyne.TextWrapWord
	a.updatedLabel = widget.NewLabel("-")
	a.connectionLabel = widget.NewLabel("Disconnected")
	a.connectionLabel.Wrapping = fyne.TextWrapWord
	a.statusLabel = widget.NewLabel("Connecting to codex-buddy")

	a.themeButton = widget.NewButtonWithIcon("Dark (T)", theme.ColorPaletteIcon(), a.toggleTheme)
	a.themeButton.Importance = widget.HighImportance
	a.refreshButton = widget.NewButtonWithIcon("Refresh (R)", theme.ViewRefreshIcon(), func() { go a.refreshNow(context.Background()) })
	a.refreshButton.Importance = widget.HighImportance
	statusSummary := container.NewVBox(
		a.titleLabel,
		a.activeLabel,
		a.summaryLabel,
	)
	stats := widget.NewCard("Current Session", "", container.NewVBox(
		statusSummary,
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			metaBlock("Updated", a.updatedLabel),
			metaBlock("Connection", a.connectionLabel),
		),
		metaBlock("Status", a.statusLabel),
	))

	header := container.NewBorder(
		nil,
		nil,
		container.NewCenter(container.NewStack(a.badgeFill, container.NewCenter(a.badgeLabel))),
		container.NewHBox(a.themeButton, a.refreshButton),
		nil,
	)

	a.sessionList = container.NewVBox(widget.NewLabel("No active sessions"))
	sidebar := widget.NewCard("Session Overview", "", a.sessionList)

	content := container.NewVBox(
		header,
		stats,
		sidebar,
	)
	a.rootScroll = container.NewVScroll(content)

	return container.NewMax(a.bgFill, container.NewPadded(a.rootScroll))
}

func metaBlock(title string, value fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabel(title),
		value,
	)
}

func (a *App) installKeyHandlers() {
	a.window.Canvas().SetOnTypedKey(func(event *fyne.KeyEvent) {
		switch event.Name {
		case fyne.KeyA:
			go a.ackPrimary()
		case fyne.KeyC:
			a.confirmContinue()
		case fyne.KeyEscape:
			a.dismissNotification()
		case fyne.KeyJ:
			a.scrollBy(88)
		case fyne.KeyK:
			a.scrollBy(-88)
		case fyne.KeyR:
			go a.refreshNow(context.Background())
		case fyne.KeyT:
			a.toggleTheme()
		}
	})
}

func (a *App) startSync(ctx context.Context) {
	go a.pollLoop(ctx)

	for {
		if ctx.Err() != nil {
			return
		}
		err := a.client.StreamStatus(ctx, func(status StatusResponse) {
			notifs, notifErr := a.client.LoadNotifications(ctx)
			if notifErr != nil {
				a.applyState(status, nil, true, notifErr)
				return
			}
			a.applyState(status, notifs, true, nil)
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil && a.logger != nil {
			a.logger.Printf("uconsole stream disconnected: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(a.cfg.ReconnectDelayMS) * time.Millisecond):
		}
	}
}

func (a *App) pollLoop(ctx context.Context) {
	_ = a.refreshNow(ctx)
	ticker := time.NewTicker(time.Duration(a.cfg.PollFallbackMS) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = a.refreshNow(ctx)
		}
	}
}

func (a *App) refreshNow(ctx context.Context) error {
	status, err := a.client.LoadStatus(ctx)
	if err != nil {
		a.applyRefreshError(err)
		return err
	}
	notifs, notifErr := a.client.LoadNotifications(ctx)
	a.applyState(status, notifs, true, notifErr)
	return notifErr
}

func (a *App) applyRefreshError(err error) {
	if err == nil {
		return
	}

	a.stateMu.RLock()
	lastSuccess := a.lastSuccess
	connected := a.connected
	a.stateMu.RUnlock()

	grace := time.Duration(a.cfg.PollFallbackMS+a.cfg.ReconnectDelayMS)*time.Millisecond + 2*time.Second
	if connected && !lastSuccess.IsZero() && time.Since(lastSuccess) < grace {
		a.stateMu.Lock()
		a.lastError = err.Error()
		a.stateMu.Unlock()
		a.setStatusLine("Refresh failed: " + briefError(err.Error(), 42))
		return
	}

	a.applyState(offlineStatus(), nil, false, err)
}

func (a *App) applyState(status StatusResponse, notifications []NotificationResponse, connected bool, err error) {
	a.stateMu.Lock()
	a.lastStatus = status
	a.lastNotifs = notifications
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
	defer a.stateMu.RUnlock()

	badgeText, badgeColor := badgeStyle(a.lastStatus.OverallState)
	a.badgeLabel.Text = strings.ToUpper(badgeText)
	a.badgeLabel.Color = color.White
	a.badgeLabel.Refresh()
	a.badgeFill.FillColor = badgeColor
	a.badgeFill.Refresh()
	a.bgFill.FillColor = appBackground(a.darkMode)
	a.bgFill.Refresh()

	a.titleLabel.SetText(titleForState(a.lastStatus.OverallState))
	a.summaryLabel.SetText(summaryForState(a.lastStatus.OverallState, a.connected, a.lastError))
	a.activeLabel.SetText(activeSessionLabel(a.lastStatus))
	a.themeButton.SetText(themeToggleLabel(a.darkMode))
	if a.lastStatus.ServerTime.IsZero() {
		a.updatedLabel.SetText("-")
	} else {
		a.updatedLabel.SetText(a.lastStatus.ServerTime.Local().Format("15:04:05"))
	}
	if a.connected {
		a.connectionLabel.SetText("Connected")
	} else {
		a.connectionLabel.SetText("Disconnected")
	}
	a.statusLabel.SetText(statusText(a.connected, a.lastError, a.statusLine, a.statusUntil))

	a.renderSessionList(a.lastStatus.Sessions)
}

func (a *App) ackPrimary() {
	a.stateMu.RLock()
	primary := a.primaryNotificationLocked()
	a.stateMu.RUnlock()
	if primary == nil {
		return
	}
	if err := a.client.AckNotification(context.Background(), primary.ID); err != nil {
		a.setStatusLine(err.Error())
		return
	}
	_ = a.refreshNow(context.Background())
}

func (a *App) confirmContinue() {
	a.stateMu.RLock()
	primary := a.primaryNotificationLocked()
	a.stateMu.RUnlock()
	if primary == nil || !canContinue(*primary) {
		return
	}

	dialog.NewConfirm("Send Continue", "Send one \"continue + Enter\" action to the current Codex session?", func(ok bool) {
		if ok {
			go a.executeContinue(*primary)
		}
	}, a.window).Show()
}

func (a *App) executeContinue(item NotificationResponse) {
	a.setStatusLine("Sending continue...")
	if err := a.client.ContinueNotification(context.Background(), item); err != nil {
		a.setStatusLine(err.Error())
		return
	}
	a.setStatusLine("Continue sent")
	_ = a.refreshNow(context.Background())
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
	if currentDialog != nil && currentDialogID == primary.ID {
		return
	}
	if shownID == primary.ID {
		return
	}
	if currentDialog != nil {
		currentDialog.Hide()
	}

	item := *primary
	sessionLabel := sessionLabelByID(status.Sessions, item.SessionID)
	dlg := a.buildNotificationDialog(item, sessionLabel)

	a.stateMu.Lock()
	a.notifDialog = dlg
	a.dialogNotifID = item.ID
	a.shownNotifID = item.ID
	a.stateMu.Unlock()

	dlg.Show()
}

func (a *App) buildNotificationDialog(item NotificationResponse, sessionLabel string) dialog.Dialog {
	title := widget.NewLabelWithStyle(cardTitle(&item), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	session := widget.NewLabel("Session: " + valueOrDash(sessionLabel))
	session.Wrapping = fyne.TextWrapWord
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
		go a.ackNotification(item.ID)
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

func (a *App) ackNotification(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	if err := a.client.AckNotification(context.Background(), id); err != nil {
		a.setStatusLine(err.Error())
		return
	}
	_ = a.refreshNow(context.Background())
}

func (a *App) confirmContinueItem(item NotificationResponse) {
	if !canContinue(item) {
		return
	}
	dialog.NewConfirm("Send Continue", "Send one \"continue + Enter\" action to the current Codex session?", func(ok bool) {
		if ok {
			go a.executeContinue(item)
		}
	}, a.window).Show()
}

func offlineStatus() StatusResponse {
	return StatusResponse{
		OverallState:  model.StateOffline,
		SessionsCount: 0,
		Sessions:      nil,
		ServerTime:    time.Now(),
	}
}

func badgeStyle(state model.State) (string, color.NRGBA) {
	switch state {
	case model.StateIdle:
		return "idle", color.NRGBA{R: 0x58, G: 0x72, B: 0x58, A: 0xFF}
	case model.StateRunning, model.StateRunningBash:
		return "running", color.NRGBA{R: 0x19, G: 0x5F, B: 0x92, A: 0xFF}
	case model.StateAttention:
		return "attention", color.NRGBA{R: 0xBC, G: 0x7A, B: 0x00, A: 0xFF}
	case model.StateError:
		return "error", color.NRGBA{R: 0xB6, G: 0x3B, B: 0x2F, A: 0xFF}
	default:
		return "offline", color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF}
	}
}

func titleForState(state model.State) string {
	switch state {
	case model.StateAttention:
		return "Needs your attention"
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
	if !connected && lastError != "" {
		return "Connection to the remote buddy is unavailable: " + lastError
	}
	switch state {
	case model.StateAttention:
		return "A run just finished. A notification dialog should appear."
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

func cardTitle(primary *NotificationResponse) string {
	if primary == nil {
		return "Primary Card"
	}
	switch primary.Kind {
	case model.NotificationError:
		return "Error Card"
	default:
		return "Attention Card"
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

func (a *App) renderSessionList(sessions []SessionResponse) {
	a.sessionList.Objects = sessionListObjects(sessions)
	a.sessionList.Refresh()
}

func sessionListObjects(sessions []SessionResponse) []fyne.CanvasObject {
	if len(sessions) == 0 {
		label := widget.NewLabel("No active sessions")
		return []fyne.CanvasObject{label}
	}
	objects := make([]fyne.CanvasObject, 0, len(sessions)*2)
	for i, session := range sessions {
		objects = append(objects, sessionRow(session))
		if i < len(sessions)-1 {
			objects = append(objects, widget.NewSeparator())
		}
	}
	return objects
}

func sessionRow(session SessionResponse) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(sessionListTitle(session), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Wrapping = fyne.TextWrapWord

	idText := session.ShortSessionID
	if strings.TrimSpace(idText) == "" {
		idText = shortSessionID(session.SessionID)
	}
	meta := widget.NewLabel(idText)

	top := container.NewBorder(
		nil,
		nil,
		container.NewVBox(title, meta),
		stateBadge(session.State),
		nil,
	)

	objects := []fyne.CanvasObject{top}
	if summary := firstNonEmptyText(session.AttentionSummary, session.Summary); summary != "" {
		summaryLabel := widget.NewLabel(summary)
		summaryLabel.Wrapping = fyne.TextWrapWord
		objects = append(objects, summaryLabel)
	}

	updated := widget.NewLabel("Updated " + relativeTime(session.UpdatedAt))
	objects = append(objects, updated)

	return container.NewVBox(objects...)
}

func stateBadge(state model.State) fyne.CanvasObject {
	label, fill := badgeStyle(state)
	bg := canvas.NewRectangle(fill)
	bg.SetMinSize(fyne.NewSize(92, 28))
	text := canvas.NewText(strings.ToUpper(label), color.White)
	text.Alignment = fyne.TextAlignCenter
	text.TextStyle = fyne.TextStyle{Bold: true}
	text.TextSize = 11
	return container.NewStack(bg, container.NewCenter(text))
}

func activeSessionLabel(status StatusResponse) string {
	if strings.TrimSpace(status.ActiveSessionDisplayTitle) != "" {
		return status.ActiveSessionDisplayTitle
	}
	if session := findSession(status.Sessions, status.ActiveSessionID); session != nil {
		return sessionLabel(*session)
	}
	return valueOrDash(status.ActiveSessionID)
}

func sessionLabelByID(sessions []SessionResponse, sessionID string) string {
	if session := findSession(sessions, sessionID); session != nil {
		return sessionLabel(*session)
	}
	return valueOrDash(sessionID)
}

func findSession(sessions []SessionResponse, sessionID string) *SessionResponse {
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			return &sessions[i]
		}
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

func shortSessionID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return valueOrDash(value)
	}
	return value[:8]
}

func relativeTime(when time.Time) string {
	if when.IsZero() {
		return "-"
	}
	delta := time.Since(when)
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

func statusText(connected bool, lastError, transient string, until time.Time) string {
	if strings.TrimSpace(transient) != "" && time.Now().Before(until) {
		return briefError(transient, 56)
	}
	if connected {
		return "Live updates"
	}
	if strings.TrimSpace(lastError) == "" {
		return "Disconnected"
	}
	return "Disconnected: " + briefError(lastError, 40)
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
	fyne.Do(a.render)
}

func (a *App) applyTheme() {
	if a.fyneApp == nil {
		return
	}
	if a.darkMode {
		a.fyneApp.Settings().SetTheme(theme.DarkTheme())
		return
	}
	a.fyneApp.Settings().SetTheme(theme.LightTheme())
}

func (a *App) dismissNotification() {
	a.stateMu.RLock()
	currentDialog := a.notifDialog
	a.stateMu.RUnlock()
	if currentDialog != nil {
		currentDialog.Hide()
	}
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

func normalizedWindowSize(width, height int) (int, int) {
	if width <= 0 {
		width = 640
	}
	if height <= 0 {
		height = 680
	}
	if width > 640 {
		width = 640
	}
	if height > 680 {
		height = 680
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
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	}
	return color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
}
