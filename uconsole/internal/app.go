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
	client      *Client
	fyneApp     fyne.App
	window      fyne.Window
	lightRunner *light.Runner
	stateMu     sync.RWMutex
	lastStatus  StatusResponse
	lastNotifs  []NotificationResponse
	connected   bool
	statusLine  string
	lastError   string
	lastSuccess time.Time
	holdMu      sync.Mutex
	holdTimer   *time.Timer
	holdActive  bool

	badgeFill       *canvas.Rectangle
	badgeLabel      *widget.Label
	titleLabel      *widget.Label
	summaryLabel    *widget.Label
	activeLabel     *widget.Label
	updatedLabel    *widget.Label
	connectionLabel *widget.Label
	statusLabel     *widget.Label
	sessionList     *widget.Label
	cardTitle       *widget.Label
	cardSession     *widget.Label
	cardSummary     *widget.Label
	cardMeta        *widget.Label
	ackButton       *widget.Button
	continueButton  *widget.Button
	refreshButton   *widget.Button
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

	gui.fyneApp = app.NewWithID("github.com.vxider.codex-buddy.uconsole")
	gui.window = gui.fyneApp.NewWindow("codex-buddy uConsole")
	gui.window.SetMaster()
	if cfg.Window.Fullscreen {
		gui.window.SetFullScreen(true)
	} else {
		gui.window.Resize(fyne.NewSize(float32(cfg.Window.Width), float32(cfg.Window.Height)))
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
	bg := canvas.NewRectangle(color.NRGBA{R: 0xF2, G: 0xEA, B: 0xD8, A: 0xFF})

	a.badgeFill = canvas.NewRectangle(color.NRGBA{R: 0x4A, G: 0x4B, B: 0x50, A: 0xFF})
	a.badgeFill.SetMinSize(fyne.NewSize(140, 36))
	a.badgeLabel = widget.NewLabel("OFFLINE")
	a.badgeLabel.Alignment = fyne.TextAlignCenter

	a.titleLabel = widget.NewLabelWithStyle("Codex companion", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	a.titleLabel.Wrapping = fyne.TextWrapWord
	a.summaryLabel = widget.NewLabel("Waiting for the codex-buddy service.")
	a.summaryLabel.Wrapping = fyne.TextWrapWord

	a.activeLabel = widget.NewLabel("-")
	a.updatedLabel = widget.NewLabel("-")
	a.connectionLabel = widget.NewLabel("Disconnected")
	a.statusLabel = widget.NewLabel("Connecting to codex-buddy")

	stats := widget.NewCard("Current Session", "", container.NewVBox(
		statRow("Active Session", a.activeLabel),
		statRow("Updated", a.updatedLabel),
		statRow("Connection", a.connectionLabel),
		statRow("Status", a.statusLabel),
	))

	header := widget.NewCard("", "", container.NewBorder(
		nil,
		nil,
		container.NewVBox(container.NewStack(a.badgeFill, container.NewCenter(a.badgeLabel))),
		stats,
		container.NewVBox(a.titleLabel, a.summaryLabel),
	))

	a.cardTitle = widget.NewLabelWithStyle("Primary Card", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	a.cardSession = widget.NewLabel("No pending notification")
	a.cardMeta = widget.NewLabel("Handle complex cases in the terminal.")
	a.cardSummary = widget.NewLabel("A compact result preview will appear here when Codex finishes a run.")
	a.cardSummary.Wrapping = fyne.TextWrapWord
	a.cardSummary.TextStyle = fyne.TextStyle{Bold: true}

	a.ackButton = widget.NewButton("Acknowledge (A)", func() { go a.ackPrimary() })
	a.continueButton = widget.NewButton("Continue", func() { a.confirmContinue() })
	a.refreshButton = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { go a.refreshNow(context.Background()) })

	cardBody := container.NewVBox(
		a.cardTitle,
		a.cardSession,
		widget.NewSeparator(),
		a.cardSummary,
		a.cardMeta,
		container.NewHBox(a.ackButton, a.continueButton, layout.NewSpacer(), a.refreshButton),
	)
	card := widget.NewCard("Notification Card", "", cardBody)

	a.sessionList = widget.NewLabel("No active sessions")
	a.sessionList.Wrapping = fyne.TextWrapWord
	sidebar := widget.NewCard("Session Overview", "", a.sessionList)

	split := container.NewHSplit(card, sidebar)
	split.SetOffset(0.68)

	root := container.NewBorder(
		header,
		nil,
		nil,
		nil,
		split,
	)

	return container.NewMax(bg, container.NewPadded(root))
}

func statRow(title string, value fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widget.NewLabel(title), nil, value)
}

func (a *App) installKeyHandlers() {
	a.window.Canvas().SetOnTypedKey(func(event *fyne.KeyEvent) {
		switch event.Name {
		case fyne.KeyA:
			go a.ackPrimary()
		case fyne.KeyR:
			go a.refreshNow(context.Background())
		}
	})

	if deskCanvas, ok := a.window.Canvas().(desktop.Canvas); ok {
		deskCanvas.SetOnKeyDown(func(event *fyne.KeyEvent) {
			if event.Name == fyne.KeyC {
				a.beginHoldContinue()
			}
		})
		deskCanvas.SetOnKeyUp(func(event *fyne.KeyEvent) {
			if event.Name == fyne.KeyC {
				a.cancelHoldContinue(false)
			}
		})
	}
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
		if err != nil {
			a.setStatusLine("Live stream disconnected, reconnecting: " + err.Error())
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
		a.applyState(offlineStatus(), nil, false, err)
		return err
	}
	notifs, notifErr := a.client.LoadNotifications(ctx)
	a.applyState(status, notifs, true, notifErr)
	return notifErr
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
		a.statusLine = err.Error()
	} else {
		a.lastError = ""
		a.statusLine = "Connected"
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

	fyne.Do(a.render)
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

	primary := a.primaryNotificationLocked()
	badgeText, badgeColor := badgeStyle(a.lastStatus.OverallState)
	a.badgeLabel.SetText(strings.ToUpper(badgeText))
	a.badgeFill.FillColor = badgeColor
	a.badgeFill.Refresh()

	a.titleLabel.SetText(titleForState(a.lastStatus.OverallState))
	a.summaryLabel.SetText(summaryForState(a.lastStatus.OverallState, a.connected, a.lastError))
	a.activeLabel.SetText(activeSessionLabel(a.lastStatus))
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
	a.statusLabel.SetText(valueOrDash(a.statusLine))

	a.cardTitle.SetText(cardTitle(primary))
	if primary == nil {
		a.cardSession.SetText("No pending notification")
		a.cardSummary.SetText("A compact result preview will appear here when Codex finishes a run.")
		a.cardMeta.SetText("Press A to acknowledge, or hold C for 800ms to send continue.")
		a.ackButton.Disable()
		a.continueButton.Disable()
	} else {
		a.cardSession.SetText("Session: " + sessionLabelByID(a.lastStatus.Sessions, primary.SessionID))
		a.cardSummary.SetText(valueOrDash(primary.Summary))
		a.cardMeta.SetText(primaryMeta(*primary))
		a.ackButton.Enable()
		if canContinue(*primary) {
			a.continueButton.Enable()
		} else {
			a.continueButton.Disable()
		}
	}

	a.sessionList.SetText(sessionListText(a.lastStatus.Sessions))
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

func (a *App) beginHoldContinue() {
	a.stateMu.RLock()
	primary := a.primaryNotificationLocked()
	a.stateMu.RUnlock()
	if primary == nil || !canContinue(*primary) {
		return
	}

	a.holdMu.Lock()
	defer a.holdMu.Unlock()
	if a.holdTimer != nil {
		return
	}
	a.holdActive = true
	a.setStatusLine("Waiting for continue hold confirmation...")
	item := *primary
	a.holdTimer = time.AfterFunc(time.Duration(a.cfg.ContinueHoldMS)*time.Millisecond, func() {
		a.holdMu.Lock()
		a.holdActive = false
		a.holdTimer = nil
		a.holdMu.Unlock()
		a.executeContinue(item)
	})
}

func (a *App) cancelHoldContinue(triggered bool) {
	a.holdMu.Lock()
	timer := a.holdTimer
	a.holdTimer = nil
	wasActive := a.holdActive
	a.holdActive = false
	a.holdMu.Unlock()
	if timer == nil {
		return
	}
	if timer.Stop() && wasActive && !triggered {
		a.setStatusLine("Continue cancelled")
	}
}

func (a *App) setStatusLine(message string) {
	a.stateMu.Lock()
	a.statusLine = message
	a.stateMu.Unlock()
	fyne.Do(a.render)
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
		return "A run just finished. The primary card will show a compact preview."
	case model.StateError:
		return "Handle complex or broken states in the terminal first."
	case model.StateRunning, model.StateRunningBash:
		return "uConsole is a sidecar for alerts, continue, and LED feedback."
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
	return "Press A to acknowledge, or hold C for 800ms to send continue."
}

func sessionListText(sessions []SessionResponse) string {
	if len(sessions) == 0 {
		return "No active sessions"
	}
	var lines []string
	for _, session := range sessions {
		line := fmt.Sprintf("• %s  [%s]", sessionListTitle(session), session.State)
		if session.ShortSessionID != "" && session.ShortSessionID != sessionListTitle(session) {
			line += "  #" + session.ShortSessionID
		}
		if summary := firstNonEmptyText(session.AttentionSummary, session.Summary); summary != "" {
			line += "\n  " + summary
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
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

func canContinue(item NotificationResponse) bool {
	for _, action := range item.Actions {
		if action == model.NotificationActionContinue {
			return true
		}
	}
	return false
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
