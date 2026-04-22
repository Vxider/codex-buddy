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
		statusLine: "正在连接 codex-buddy",
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
	a.summaryLabel = widget.NewLabel("等待连接 codex-buddy 服务。")
	a.summaryLabel.Wrapping = fyne.TextWrapWord

	a.activeLabel = widget.NewLabel("-")
	a.updatedLabel = widget.NewLabel("-")
	a.connectionLabel = widget.NewLabel("未连接")
	a.statusLabel = widget.NewLabel("正在连接 codex-buddy")

	stats := widget.NewCard("当前会话", "", container.NewVBox(
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
	a.cardSession = widget.NewLabel("暂无待确认通知")
	a.cardMeta = widget.NewLabel("复杂情况请回 terminal 处理。")
	a.cardSummary = widget.NewLabel("Codex 运行结束后，这里会显示一条压缩后的结果预览。")
	a.cardSummary.Wrapping = fyne.TextWrapWord
	a.cardSummary.TextStyle = fyne.TextStyle{Bold: true}

	a.ackButton = widget.NewButton("已读 (A)", func() { go a.ackPrimary() })
	a.continueButton = widget.NewButton("继续", func() { a.confirmContinue() })
	a.refreshButton = widget.NewButtonWithIcon("刷新", theme.ViewRefreshIcon(), func() { go a.refreshNow(context.Background()) })

	cardBody := container.NewVBox(
		a.cardTitle,
		a.cardSession,
		widget.NewSeparator(),
		a.cardSummary,
		a.cardMeta,
		container.NewHBox(a.ackButton, a.continueButton, layout.NewSpacer(), a.refreshButton),
	)
	card := widget.NewCard("通知卡片", "", cardBody)

	a.sessionList = widget.NewLabel("暂无活跃 session")
	a.sessionList.Wrapping = fyne.TextWrapWord
	sidebar := widget.NewCard("会话概览", "", a.sessionList)

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
			a.setStatusLine("实时流断开，正在重连: " + err.Error())
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
		a.statusLine = "已连接"
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
		a.cardSession.SetText("暂无待确认通知")
		a.cardSummary.SetText("Codex 运行结束后，这里会显示一条压缩后的结果预览。")
		a.cardMeta.SetText("A 已读，长按键盘 C 800ms 发送继续。")
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

	dialog.NewConfirm("发送继续", "向当前 Codex 会话发送一次“继续 + Enter”吗？", func(ok bool) {
		if ok {
			go a.executeContinue(*primary)
		}
	}, a.window).Show()
}

func (a *App) executeContinue(item NotificationResponse) {
	a.setStatusLine("正在发送继续...")
	if err := a.client.ContinueNotification(context.Background(), item); err != nil {
		a.setStatusLine(err.Error())
		return
	}
	a.setStatusLine("继续已发送")
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
	a.setStatusLine("继续确认中...")
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
		a.setStatusLine("已取消继续")
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
		return "需要你看一眼"
	case model.StateError:
		return "本轮出现错误"
	case model.StateRunning, model.StateRunningBash:
		return "Codex 正在工作"
	case model.StateIdle:
		return "当前空闲"
	default:
		return "Codex companion"
	}
}

func summaryForState(state model.State, connected bool, lastError string) string {
	if !connected && lastError != "" {
		return "与远端 buddy 的连接不可用: " + lastError
	}
	switch state {
	case model.StateAttention:
		return "任务刚完成，主卡片会显示一条压缩预览。"
	case model.StateError:
		return "优先回 terminal 处理复杂情况。"
	case model.StateRunning, model.StateRunningBash:
		return "uConsole 只做旁路提醒、继续和灯效。"
	case model.StateIdle:
		return "当前没有正在执行的任务。"
	default:
		return "等待远端 codex-buddy 状态。"
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
		return "错误提醒：建议回 terminal 处理。"
	}
	if primary.State == model.NotificationAcked {
		return "这条通知已读，但仍可继续。"
	}
	return "A 已读，长按键盘 C 800ms 发送继续。"
}

func sessionListText(sessions []SessionResponse) string {
	if len(sessions) == 0 {
		return "暂无活跃 session"
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
