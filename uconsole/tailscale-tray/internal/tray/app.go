package tray

import (
	"context"
	"image/color"
	"log"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/vxider/codex-buddy/uconsole/tailscale-tray/internal/tailscalecli"
)

type App struct {
	logger *log.Logger

	ctx    context.Context
	cancel context.CancelFunc

	mu            sync.Mutex
	state         tailscalecli.State
	busy          bool
	lastError     string
	exitNodeItems []*systray.MenuItem

	statusItem      *systray.MenuItem
	exitItem        *systray.MenuItem
	errorItem       *systray.MenuItem
	disableExitItem *systray.MenuItem
	exitNodesMenu   *systray.MenuItem
	refreshItem     *systray.MenuItem
	quitItem        *systray.MenuItem
}

func Run(logger *log.Logger) {
	ctx, cancel := context.WithCancel(context.Background())
	app := &App{
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}
	systray.Run(app.onReady, app.onExit)
}

func (a *App) onReady() {
	a.statusItem = systray.AddMenuItem("Tailscale: loading...", "")
	a.statusItem.Disable()
	a.exitItem = systray.AddMenuItem("Exit Node: -", "")
	a.exitItem.Disable()
	a.errorItem = systray.AddMenuItem("", "")
	a.errorItem.Disable()
	a.errorItem.Hide()

	systray.AddSeparator()
	a.disableExitItem = systray.AddMenuItem("Disable Exit Node", "Disable the current exit node")
	a.exitNodesMenu = systray.AddMenuItem("Exit Nodes", "Available exit nodes")

	systray.AddSeparator()
	a.refreshItem = systray.AddMenuItem("Refresh", "Refresh Tailscale status")
	a.quitItem = systray.AddMenuItem("Quit", "Quit the tray app")

	go a.watchRefresh()
	go a.watchQuit()
	go a.watchDisableExit()
	go a.watchTrayOpen()
	go a.pollLoop()

	a.refresh()
}

func (a *App) onExit() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) watchRefresh() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.refreshItem.ClickedCh:
			a.refresh()
		}
	}
}

func (a *App) watchQuit() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.quitItem.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *App) watchDisableExit() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.disableExitItem.ClickedCh:
			a.applyExitNode("")
		}
	}
}

func (a *App) watchTrayOpen() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-systray.TrayOpenedCh:
			a.refresh()
		}
	}
}

func (a *App) pollLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.refresh()
		}
	}
}

func (a *App) refresh() {
	ctx, cancel := context.WithTimeout(a.ctx, 4*time.Second)
	defer cancel()

	state := tailscalecli.Load(ctx)

	a.mu.Lock()
	a.state = state
	if state.Error != "" {
		a.lastError = state.Error
	}
	busy := a.busy
	a.mu.Unlock()

	a.render(state, busy)
}

func (a *App) applyExitNode(target string) {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return
	}
	a.busy = true
	state := a.state
	a.mu.Unlock()

	a.render(state, true)

	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
		defer cancel()

		err := tailscalecli.SetExitNode(ctx, target)
		a.mu.Lock()
		a.busy = false
		if err != nil {
			a.lastError = err.Error()
		}
		a.mu.Unlock()

		a.refresh()
	}()
}

func (a *App) render(state tailscalecli.State, busy bool) {
	icon := trayIcon(trayColor(state, busy))
	systray.SetTemplateIcon(icon, icon)
	systray.SetTitle(trayTitle(state, busy))
	systray.SetTooltip(trayTooltip(state, busy))

	a.statusItem.SetTitle(statusMenuLabel(state, busy))
	a.exitItem.SetTitle(exitMenuLabel(state))

	if busy {
		a.disableExitItem.Disable()
		a.refreshItem.Disable()
	} else {
		a.refreshItem.Enable()
	}

	if state.ExitNodeName == "" && state.ExitNodeIP == "" {
		a.disableExitItem.Disable()
	} else if !busy {
		a.disableExitItem.Enable()
	}

	if state.Error != "" {
		a.errorItem.SetTitle(compactError(state.Error))
		a.errorItem.Show()
	} else if a.lastError != "" {
		a.errorItem.SetTitle(compactError(a.lastError))
		a.errorItem.Show()
	} else {
		a.errorItem.Hide()
	}

	a.renderExitNodes(state, busy)
}

func (a *App) renderExitNodes(state tailscalecli.State, busy bool) {
	a.mu.Lock()
	oldItems := append([]*systray.MenuItem(nil), a.exitNodeItems...)
	a.exitNodeItems = nil
	a.mu.Unlock()

	for _, item := range oldItems {
		item.Remove()
	}

	var newItems []*systray.MenuItem
	switch {
	case !state.Installed:
		item := a.exitNodesMenu.AddSubMenuItem("tailscale not installed", "")
		item.Disable()
		newItems = append(newItems, item)
	case state.Error != "" && len(state.ExitNodes) == 0:
		item := a.exitNodesMenu.AddSubMenuItem("unavailable", "")
		item.Disable()
		newItems = append(newItems, item)
	case len(state.ExitNodes) == 0:
		item := a.exitNodesMenu.AddSubMenuItem("no exit nodes", "")
		item.Disable()
		newItems = append(newItems, item)
	default:
		for _, node := range state.ExitNodes {
			node := node
			item := a.exitNodesMenu.AddSubMenuItemCheckbox(exitNodeLabel(node), "", node.Current)
			if busy || (!node.Online && !node.Current) {
				item.Disable()
			}
			go func(item *systray.MenuItem, node tailscalecli.ExitNode) {
				for range item.ClickedCh {
					if node.Current {
						a.applyExitNode("")
						return
					}
					target := strings.TrimSpace(node.IP)
					if target == "" {
						target = strings.TrimSpace(node.Name)
					}
					a.applyExitNode(target)
					return
				}
			}(item, node)
			newItems = append(newItems, item)
		}
	}

	a.mu.Lock()
	a.exitNodeItems = newItems
	a.mu.Unlock()
}

func trayColor(state tailscalecli.State, busy bool) color.NRGBA {
	switch {
	case busy:
		return color.NRGBA{R: 0x2F, G: 0x78, B: 0xD6, A: 0xFF}
	case !state.Installed:
		return color.NRGBA{R: 0x5B, G: 0x63, B: 0x6E, A: 0xFF}
	case state.Error != "":
		return color.NRGBA{R: 0xC7, G: 0x83, B: 0x19, A: 0xFF}
	case state.Online:
		return color.NRGBA{R: 0x2D, G: 0x9A, B: 0x5F, A: 0xFF}
	default:
		return color.NRGBA{R: 0x6B, G: 0x72, B: 0x79, A: 0xFF}
	}
}

func trayTitle(state tailscalecli.State, busy bool) string {
	switch {
	case busy:
		return "TS ..."
	case !state.Installed:
		return "TS -"
	case state.Error != "":
		return "TS !"
	case state.Online:
		return "TS on"
	default:
		return "TS off"
	}
}

func trayTooltip(state tailscalecli.State, busy bool) string {
	switch {
	case busy:
		return "Applying Tailscale change..."
	case !state.Installed:
		return "tailscale CLI not found"
	case state.Error != "":
		return compactError(state.Error)
	case state.ExitNodeName != "":
		return "Online via exit node: " + state.ExitNodeName
	case state.Online:
		return "Tailscale online"
	default:
		return "Tailscale offline"
	}
}

func statusMenuLabel(state tailscalecli.State, busy bool) string {
	switch {
	case busy:
		return "Tailscale: applying..."
	case !state.Installed:
		return "Tailscale: not installed"
	case state.Error != "":
		return "Tailscale: unavailable"
	case state.Online:
		return "Tailscale: online"
	default:
		return "Tailscale: offline"
	}
}

func exitMenuLabel(state tailscalecli.State) string {
	name := strings.TrimSpace(state.ExitNodeName)
	if name == "" {
		name = strings.TrimSpace(state.ExitNodeIP)
	}
	if name == "" {
		return "Exit Node: off"
	}
	return "Exit Node: " + name
}

func exitNodeLabel(node tailscalecli.ExitNode) string {
	label := strings.TrimSpace(node.Name)
	if label == "" {
		label = strings.TrimSpace(node.IP)
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

func compactError(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if len(message) > 72 {
		return message[:69] + "..."
	}
	if message == "" {
		return "unknown error"
	}
	return message
}
