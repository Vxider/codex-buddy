package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vxider/codex-buddy/internal/model"
)

type tuiPane int

const (
	tuiPaneOpenSessions tuiPane = iota
	tuiPaneSessions
)

type refreshMsg struct {
	snapshots []serverSnapshot
	at        time.Time
}

type refreshErrMsg struct {
	err error
	at  time.Time
}

type actionMsg struct {
	message string
	err     error
	refresh bool
}

type refreshTickMsg time.Time

type sessionChoice struct {
	Session sessionResponse
	Client  sourceClient
}

type notificationChoice struct {
	Notification notificationResponse
	Client       sourceClient
}

type bubbleModel struct {
	ctx context.Context
	app *App

	width  int
	height int

	activePane tuiPane
}

var (
	pageStyle = lipgloss.NewStyle().
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	activePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("12")).
			Padding(0, 1)

	inactivePaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("8")).
				Padding(0, 1)

	badgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	metaBadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	serverStripStyle = lipgloss.NewStyle().
				PaddingTop(0).
				PaddingBottom(1)

	selectedLineStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("11"))

	sessionCardStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("8")).
				Padding(0, 1)

	sessionCardActiveStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("11")).
				Padding(0, 1)

	dimLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	errorLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))

	attentionLineStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("11"))

	runningLineStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10"))

	idleLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10"))
)

func (a *App) runTUI(ctx context.Context) error {
	model := bubbleModel{
		ctx:        ctx,
		app:        a,
		activePane: tuiPaneSessions,
	}

	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func (m bubbleModel) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), refreshTickCmd(m.refreshInterval()))
}

func (m bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case refreshMsg:
		m.app.snapshots = msg.snapshots
		m.app.lastRefresh = msg.at
		m.app.clampSelections()
		return m, nil

	case refreshErrMsg:
		m.app.lastRefresh = msg.at
		m.app.setFlash("refresh failed: " + brief(msg.err.Error(), 96))
		return m, nil

	case refreshTickMsg:
		return m, tea.Batch(refreshTickCmd(m.refreshInterval()), m.refreshCmd())

	case actionMsg:
		if msg.err != nil {
			m.app.setFlash(msg.err.Error())
			return m, nil
		}
		if strings.TrimSpace(msg.message) != "" {
			m.app.setFlash(msg.message)
		}
		if msg.refresh {
			return m, m.refreshCmd()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab", "right", "l":
			m.activePane = nextVisiblePane(m.activePane, m.app.visiblePanes(), 1)
			return m, nil
		case "shift+tab", "left", "h":
			m.activePane = nextVisiblePane(m.activePane, m.app.visiblePanes(), -1)
			return m, nil
		case "up", "k":
			m.app.moveSelection(m.activePane, -1)
			return m, nil
		case "down", "j":
			m.app.moveSelection(m.activePane, 1)
			return m, nil
		case "g", "home":
			m.app.moveSelectionTo(m.activePane, 0)
			return m, nil
		case "G", "end":
			m.app.moveSelectionTo(m.activePane, 1<<30)
			return m, nil
		case "r":
			return m, m.refreshCmd()
		case "v":
			return m, m.toggleServerCmd()
		case "c", "enter":
			return m, m.continueCmd()
		}
	}

	return m, nil
}

func (m bubbleModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return pageStyle.Render("Loading terminal...")
	}
	width := m.width
	height := m.height

	header := titleStyle.Render("codex-buddy cli")
	topBar := m.renderTopBar(width)
	serverStrip := m.renderServerStrip(width)

	openItems := m.app.openSessionItems()
	contentWidth := maxInt(24, width-6)

	status := m.app.flashMessage()
	if status == "" {
		status = "Tab/h/l switch pane | j/k move | Enter/c continue | v toggle server | r refresh | q quit"
		status = dimLineStyle.Render(clipLine(status, width-4))
	} else {
		status = statusBarStyle.Render(clipLine(status, width-4))
	}

	fixedHeight := lipgloss.Height(header) + lipgloss.Height(topBar) + lipgloss.Height(serverStrip) + lipgloss.Height(status)
	availableHeight := height - fixedHeight
	if availableHeight < 4 {
		return pageStyle.Width(width).Render(
			lipgloss.JoinVertical(
				lipgloss.Left,
				header,
				topBar,
				status,
			),
		)
	}

	openMinHeight := 0
	if len(openItems) > 0 {
		openMinHeight = 7
	}
	sessionMinHeight := 6
	if availableHeight < openMinHeight+sessionMinHeight {
		switch {
		case m.activePane == tuiPaneOpenSessions && len(openItems) > 0:
			return pageStyle.Width(width).Render(
				lipgloss.JoinVertical(
					lipgloss.Left,
					header,
					topBar,
					serverStrip,
					m.renderOpenSessionsPane(contentWidth, availableHeight),
					status,
				),
			)
		default:
			return pageStyle.Width(width).Render(
				lipgloss.JoinVertical(
					lipgloss.Left,
					header,
					topBar,
					serverStrip,
					m.renderSessionsPane(contentWidth, availableHeight),
					status,
				),
			)
		}
	}

	openHeight := 0
	if len(openItems) > 0 {
		openHeight = minInt(openPaneHeight(len(openItems)), maxInt(openMinHeight, availableHeight/2))
	}
	sessionHeight := availableHeight - openHeight

	for sessionHeight < sessionMinHeight {
		if openHeight > openMinHeight {
			openHeight--
			sessionHeight++
			continue
		}
		break
	}

	sections := make([]string, 0, 3)
	if len(openItems) > 0 {
		sections = append(sections, m.renderOpenSessionsPane(contentWidth, openHeight))
	}
	sections = append(sections, m.renderSessionsPane(contentWidth, sessionHeight))
	layout := lipgloss.JoinVertical(lipgloss.Left, sections...)

	return pageStyle.Width(width).Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			topBar,
			serverStrip,
			layout,
			status,
		),
	)
}

func (m bubbleModel) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		at := time.Now()
		snapshots := m.app.loadAll(m.ctx)
		return refreshMsg{
			snapshots: snapshots,
			at:        at,
		}
	}
}

func refreshTickCmd(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		interval = 1500 * time.Millisecond
	}
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return refreshTickMsg(t)
	})
}

func (m bubbleModel) refreshInterval() time.Duration {
	interval := time.Duration(m.app.cfg.UConsole.PollFallbackMS) * time.Millisecond
	if interval < 1200*time.Millisecond {
		interval = 1200 * time.Millisecond
	}
	return interval
}

func (m bubbleModel) toggleServerCmd() tea.Cmd {
	return func() tea.Msg {
		if err := m.app.toggleServer(m.ctx); err != nil {
			return actionMsg{err: err}
		}
		state := "Local server off"
		if m.app.server.Running() {
			state = "Local server on"
		}
		return actionMsg{message: state, refresh: true}
	}
}

func (m bubbleModel) continueCmd() tea.Cmd {
	if m.activePane != tuiPaneSessions && m.activePane != tuiPaneOpenSessions {
		return func() tea.Msg {
			return actionMsg{err: fmt.Errorf("switch to Sessions before continue")}
		}
	}
	session, client, ok := m.app.selectedContinueSessionItem(m.activePane)
	if !ok {
		return func() tea.Msg {
			return actionMsg{err: fmt.Errorf("no session selected")}
		}
	}
	if !session.CanContinue || session.ContinueAction == nil {
		return func() tea.Msg {
			return actionMsg{err: fmt.Errorf("session continue is unavailable")}
		}
	}
	return func() tea.Msg {
		if err := client.ContinueSession(m.ctx, session); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{message: "Continue sent", refresh: true}
	}
}

func (m bubbleModel) summaryLine() string {
	overall := strings.ToUpper(string(aggregateState(m.app.snapshots)))
	last := "-"
	if !m.app.lastRefresh.IsZero() {
		last = m.app.lastRefresh.Local().Format("15:04:05")
	}
	return fmt.Sprintf(
		"overall %s | local server %s | listen http://%s | refreshed %s",
		overall,
		strings.ToUpper(strings.ReplaceAll(m.app.serverSummary(), "(", " (")),
		m.app.cfg.Listen.Address(),
		last,
	)
}

func (m bubbleModel) renderTopBar(width int) string {
	left := []string{
		stateBadge(strings.ToUpper(string(aggregateState(m.app.snapshots))), aggregateState(m.app.snapshots)),
		serverToggleBadge(m.app.server.Running()),
	}
	right := []string{
		metaBadgeStyle.Background(lipgloss.Color("8")).Render("listen " + m.app.cfg.Listen.Address()),
		metaBadgeStyle.Background(lipgloss.Color("8")).Render("updated " + m.updatedAtText()),
	}
	parts := append(left, right...)
	return strings.Join(parts, " ")
}

func (m bubbleModel) renderServerStrip(width int) string {
	chips := make([]string, 0, len(m.app.snapshots))
	if len(m.app.snapshots) == 0 {
		chips = append(chips, serverChip("No servers", "", false))
	} else {
		for _, snapshot := range m.app.snapshots {
			chips = append(chips, serverChip(snapshot.Target.Name, snapshot.Target.ID, snapshot.Connected))
		}
	}
	_ = width
	return serverStripStyle.Render(strings.Join(chips, " "))
}

func (m bubbleModel) renderOpenSessionsPane(width int, height int) string {
	items := m.app.openSessionItems()
	lines := []string{dimLineStyle.Render("Continue-eligible attention sessions")}
	limit := maxInt(1, (innerPaneHeight(height)-1)/sessionCardHeight())
	start, end := visibleWindow(m.app.selectedOpenSessionIndex, len(items), visibleLimit(len(items), limit))
	for i := start; i < end; i++ {
		lines = append(lines, m.renderSessionCard(items[i].Session, width-6, m.activePane == tuiPaneOpenSessions, i == m.app.selectedOpenSessionIndex, true))
	}
	return renderPane("Open Sessions", m.activePane == tuiPaneOpenSessions, width, height, lines)
}

func (m bubbleModel) renderSessionsPane(width int, height int) string {
	items := m.app.otherSessionItems()
	if len(items) == 0 {
		return renderPane("Sessions", m.activePane == tuiPaneSessions, width, height, []string{dimLineStyle.Render("No other sessions")})
	}

	cols := 1
	if width >= 96 {
		cols = 2
	}
	cardWidth := width - 4
	if cols == 2 {
		cardWidth = (width - 7) / 2
	}

	rows := maxInt(1, innerPaneHeight(height)/sessionCardHeight())
	start, end := visibleWindow(m.app.selectedSessionIndex, len(items), visibleLimit(len(items), rows*cols))
	cards := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		cards = append(cards, m.renderSessionCard(items[i].Session, cardWidth, m.activePane == tuiPaneSessions, i == m.app.selectedSessionIndex, false))
	}
	lines := renderCardGrid(cards, cols, cardWidth)
	return renderPane("Sessions", m.activePane == tuiPaneSessions, width, height, lines)
}

func renderPane(title string, active bool, width int, height int, lines []string) string {
	style := inactivePaneStyle
	if active {
		style = activePaneStyle
	}
	if width < 20 {
		width = 20
	}
	if height < 4 {
		height = 4
	}
	innerWidth := maxInt(1, width-paneFrameWidth())
	body := strings.Join(fitPaneBody(lines, innerPaneHeight(height)), "\n")
	header := title
	if active {
		header = titleStyle.Render(title)
	} else {
		header = subtitleStyle.Render(title)
	}
	return style.Width(innerWidth).Render(header + "\n" + body)
}

func (m bubbleModel) renderSessionCard(session sessionResponse, width int, activePane bool, selected bool, open bool) string {
	cardStyle := sessionCardStyle
	if activePane && selected {
		cardStyle = sessionCardActiveStyle
	}
	if width < 24 {
		width = 24
	}
	innerWidth := maxInt(1, width-cardFrameWidth())

	title := firstNonEmpty(session.DisplayTitle, session.ShortSessionID, session.SessionID)
	header := clipLine(selectedMarker(activePane, selected)+" "+title, innerWidth)

	badges := []string{
		sourceBadge(session.ServerName, session.ServerID),
		sessionStateBadge(session.State),
	}
	if open {
		badges = append(badges, attentionBadgeStyle().Render("CONTINUE"))
	} else if session.CanContinue {
		badges = append(badges, metaBadgeStyle.Background(lipgloss.Color("10")).Render("READY"))
	}

	summary := firstNonEmpty(session.OpenSummary, session.Summary)
	if summary == "" {
		summary = "No summary"
	}
	meta := dimLineStyle.Render("Updated " + session.UpdatedAt.Local().Format("01-02 15:04"))

	body := []string{
		titleStyle.Render(header),
		strings.Join(badges, " "),
		dimLineStyle.Render(clipLine(brief(summary, maxInt(18, innerWidth*2)), innerWidth)),
		meta,
	}

	return cardStyle.Width(innerWidth).Render(strings.Join(body, "\n"))
}

func sourceBadge(name, serverID string) string {
	base := strings.ToUpper(brief(name, 14))
	if base == "" {
		base = "SERVER"
	}
	if serverID == localServerID {
		return metaBadgeStyle.Background(lipgloss.Color("6")).Render(base)
	}
	return warningBadgeStyle().Render(base)
}

func sessionStateBadge(state model.State) string {
	label := strings.ToUpper(string(state))
	if label == "" {
		label = "UNKNOWN"
	}
	color := lipgloss.Color("8")
	switch state {
	case model.StateError:
		color = lipgloss.Color("9")
	case model.StateAttention:
		return attentionBadgeStyle().Render(label)
	case model.StateRun, model.StateRunning, model.StateRunningBash:
		color = lipgloss.Color("10")
	case model.StateIdle:
		color = lipgloss.Color("12")
	}
	return metaBadgeStyle.Background(color).Render(label)
}

func renderCardGrid(cards []string, cols int, cardWidth int) []string {
	if len(cards) == 0 {
		return nil
	}
	if cols < 2 {
		return cards
	}

	lines := make([]string, 0, len(cards))
	for start := 0; start < len(cards); start += cols {
		end := start + cols
		if end > len(cards) {
			end = len(cards)
		}
		row := cards[start:end]
		maxHeight := 0
		for _, card := range row {
			if h := lipgloss.Height(card); h > maxHeight {
				maxHeight = h
			}
		}

		normalized := make([]string, 0, len(row))
		for _, card := range row {
			normalized = append(normalized, padCardHeight(card, cardWidth, maxHeight))
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, normalized...))
	}
	return lines
}

func padCardHeight(card string, width int, height int) string {
	current := lipgloss.Height(card)
	if current >= height {
		return card
	}
	fill := strings.Repeat("\n", height-current)
	return lipgloss.NewStyle().Width(width).Render(card + fill)
}

func colorForState(state model.State, connected bool) lipgloss.Style {
	if !connected {
		return dimLineStyle
	}
	switch state {
	case model.StateError:
		return errorLineStyle
	case model.StateAttention:
		return attentionLineStyle
	case model.StateRun, model.StateRunning, model.StateRunningBash:
		return runningLineStyle
	case model.StateIdle:
		return idleLineStyle
	default:
		return dimLineStyle
	}
}

func selectedMarker(active, selected bool) string {
	if active && selected {
		return selectedLineStyle.Render(">")
	}
	if selected {
		return subtitleStyle.Render(">")
	}
	return " "
}

func nextVisiblePane(current tuiPane, panes []tuiPane, delta int) tuiPane {
	if len(panes) == 0 {
		return tuiPaneSessions
	}
	index := 0
	for i, pane := range panes {
		if pane == current {
			index = i
			break
		}
	}
	index += delta
	if index < 0 {
		index = len(panes) - 1
	}
	if index >= len(panes) {
		index = 0
	}
	return panes[index]
}

func visibleWindow(selected, total, limit int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if limit <= 0 || total <= limit {
		return 0, total
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
	}
	start := selected - limit/2
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > total {
		end = total
		start = end - limit
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func clipLine(value string, width int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	if width <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

func stateBadge(label string, state model.State) string {
	color := "8"
	switch state {
	case model.StateError:
		color = "9"
	case model.StateAttention:
		return attentionStateBadgeStyle().Render(label)
	case model.StateRun, model.StateRunning, model.StateRunningBash:
		color = "10"
	case model.StateIdle:
		color = "12"
	}
	return badgeStyle.Background(lipgloss.Color(color)).Render(label)
}

func serverToggleBadge(running bool) string {
	if running {
		return badgeStyle.Background(lipgloss.Color("10")).Render("SERVER ON")
	}
	return badgeStyle.Background(lipgloss.Color("8")).Render("SERVER OFF")
}

func serverChip(name, serverID string, connected bool) string {
	base := strings.ToUpper(brief(name, 18))
	if !connected {
		return metaBadgeStyle.Background(lipgloss.Color("8")).Render(base)
	}
	if serverID == localServerID {
		return metaBadgeStyle.Background(lipgloss.Color("6")).Render(base)
	}
	return warningBadgeStyle().Render(base)
}

func (m bubbleModel) updatedAtText() string {
	if m.app.lastRefresh.IsZero() {
		return "-"
	}
	return m.app.lastRefresh.Local().Format("15:04:05")
}

func paneFrameHeight() int {
	return 2
}

func paneFrameWidth() int {
	return 4
}

func cardFrameHeight() int {
	return 2
}

func cardFrameWidth() int {
	return 4
}

func sessionCardHeight() int {
	return 4 + cardFrameHeight()
}

func innerPaneHeight(height int) int {
	return maxInt(1, height-paneFrameHeight()-1)
}

func openPaneHeight(count int) int {
	if count <= 0 {
		return 0
	}
	return paneFrameHeight() + 1 + 1 + count*sessionCardHeight()
}

func fitPaneBody(blocks []string, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}

	flat := make([]string, 0, maxLines)
	for _, block := range blocks {
		if len(flat) >= maxLines {
			break
		}
		blockLines := strings.Split(block, "\n")
		remaining := maxLines - len(flat)
		if len(blockLines) > remaining {
			blockLines = blockLines[:remaining]
		}
		flat = append(flat, blockLines...)
	}

	for len(flat) < maxLines {
		flat = append(flat, "")
	}
	return flat
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func visibleLimit(total, cap int) int {
	if total < cap {
		return total
	}
	return cap
}

func (a *App) clampSelections() {
	openSessions := a.openSessionItems()
	if len(openSessions) == 0 {
		a.selectedOpenSessionIndex = 0
	} else if a.selectedOpenSessionIndex >= len(openSessions) {
		a.selectedOpenSessionIndex = len(openSessions) - 1
	}

	sessions := a.otherSessionItems()
	if len(sessions) == 0 {
		a.selectedSessionIndex = 0
	} else if a.selectedSessionIndex >= len(sessions) {
		a.selectedSessionIndex = len(sessions) - 1
	}

}

func (a *App) moveSelection(pane tuiPane, delta int) {
	switch pane {
	case tuiPaneOpenSessions:
		a.selectedOpenSessionIndex = clampIndex(a.selectedOpenSessionIndex+delta, len(a.openSessionItems()))
	case tuiPaneSessions:
		a.selectedSessionIndex = clampIndex(a.selectedSessionIndex+delta, len(a.otherSessionItems()))
	}
}

func (a *App) moveSelectionTo(pane tuiPane, index int) {
	switch pane {
	case tuiPaneOpenSessions:
		a.selectedOpenSessionIndex = clampIndex(index, len(a.openSessionItems()))
	case tuiPaneSessions:
		a.selectedSessionIndex = clampIndex(index, len(a.otherSessionItems()))
	}
}

func clampIndex(index, size int) int {
	if size <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= size {
		return size - 1
	}
	return index
}

func (a *App) selectedSessionItem() (sessionResponse, sourceClient, bool) {
	items := a.otherSessionItems()
	if len(items) == 0 {
		return sessionResponse{}, nil, false
	}
	item := items[clampIndex(a.selectedSessionIndex, len(items))]
	return item.Session, item.Client, true
}

func (a *App) selectedOpenSessionItem() (sessionResponse, sourceClient, bool) {
	items := a.openSessionItems()
	if len(items) == 0 {
		return sessionResponse{}, nil, false
	}
	item := items[clampIndex(a.selectedOpenSessionIndex, len(items))]
	return item.Session, item.Client, true
}

func (a *App) selectedContinueSessionItem(pane tuiPane) (sessionResponse, sourceClient, bool) {
	if pane == tuiPaneOpenSessions {
		return a.selectedOpenSessionItem()
	}
	return a.selectedSessionItem()
}

func (a *App) sessionsWithClient() []sessionChoice {
	items := make([]sessionChoice, 0, 16)
	for _, snapshot := range a.snapshots {
		for _, session := range snapshot.Status.Sessions {
			items = append(items, sessionChoice{
				Session: session,
				Client:  snapshot.Target.Client,
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if stateRank(items[i].Session.State) != stateRank(items[j].Session.State) {
			return stateRank(items[i].Session.State) > stateRank(items[j].Session.State)
		}
		return items[i].Session.UpdatedAt.After(items[j].Session.UpdatedAt)
	})
	return items
}

func (a *App) openSessionItems() []sessionChoice {
	items := a.sessionsWithClient()
	out := make([]sessionChoice, 0, len(items))
	for _, item := range items {
		if item.Session.State == model.StateAttention {
			out = append(out, item)
		}
	}
	return out
}

func (a *App) otherSessionItems() []sessionChoice {
	items := a.sessionsWithClient()
	out := make([]sessionChoice, 0, len(items))
	for _, item := range items {
		if item.Session.State != model.StateAttention {
			out = append(out, item)
		}
	}
	return out
}

func (a *App) visiblePanes() []tuiPane {
	panes := make([]tuiPane, 0, 2)
	if len(a.openSessionItems()) > 0 {
		panes = append(panes, tuiPaneOpenSessions)
	}
	panes = append(panes, tuiPaneSessions)
	return panes
}

func warningBadgeStyle() lipgloss.Style {
	return metaBadgeStyle.Background(lipgloss.Color("3")).Foreground(lipgloss.Color("12"))
}

func attentionBadgeStyle() lipgloss.Style {
	return metaBadgeStyle.Background(lipgloss.Color("11")).Foreground(lipgloss.Color("12"))
}

func attentionStateBadgeStyle() lipgloss.Style {
	return badgeStyle.Background(lipgloss.Color("11")).Foreground(lipgloss.Color("12"))
}

func (a *App) notificationsWithClient() []notificationChoice {
	items := make([]notificationChoice, 0, 16)
	for _, snapshot := range a.snapshots {
		for _, item := range snapshot.Notifications {
			items = append(items, notificationChoice{
				Notification: item,
				Client:       snapshot.Target.Client,
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Notification.UpdatedAt.After(items[j].Notification.UpdatedAt)
	})
	return items
}

func (a *App) setFlash(message string) {
	a.flash = brief(strings.TrimSpace(message), 120)
	a.flashUntil = time.Now().Add(3 * time.Second)
}

func (a *App) flashMessage() string {
	if time.Now().After(a.flashUntil) {
		return ""
	}
	return a.flash
}
