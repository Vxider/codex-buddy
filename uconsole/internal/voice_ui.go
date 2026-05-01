//go:build uconsole_gui

package uconsole

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (a *App) startVoiceShortcutHold(key fyne.KeyName, actionKey string, session SessionResponse) {
	capture, err := startVoiceCapture(session)
	if err != nil {
		a.setStatusLine("Voice start failed: " + briefError(err.Error(), 80))
		return
	}

	a.stateMu.Lock()
	a.openHoldStop = nil
	a.openHoldKey = key
	a.openHoldActionKey = actionKey
	a.openHoldProgress = 1
	a.voiceCapture = capture
	a.stateMu.Unlock()

	showVoiceRecordingNotification()
	a.setStatusLine("Recording voice for " + sessionListTitle(session) + "...")
	fyne.Do(a.render)
}

func (a *App) finishVoiceShortcutHold(key fyne.KeyName, actionKey string) {
	a.stateMu.Lock()
	if a.openHoldKey != key || a.openHoldActionKey != actionKey {
		a.stateMu.Unlock()
		return
	}
	capture := a.voiceCapture
	a.voiceCapture = nil
	a.openHoldStop = nil
	a.openHoldKey = ""
	a.openHoldActionKey = ""
	a.openHoldProgress = 0
	a.stateMu.Unlock()
	fyne.Do(a.render)

	if capture == nil {
		return
	}
	if !a.startOpenSessionActionPending(actionKey) {
		return
	}
	go a.transcribeVoiceCapture(actionKey, capture.session, capture)
}

func (a *App) startVoiceClick(actionKey string, session SessionResponse) {
	if a.hasBlockingDialog() {
		return
	}

	a.stateMu.Lock()
	if a.voiceCapture != nil || a.openActionPending[actionKey] {
		a.stateMu.Unlock()
		return
	}
	a.stateMu.Unlock()

	capture, err := startVoiceCapture(session)
	if err != nil {
		showVoiceTimedNotification("uconsole voice", "录音启动失败", 0, 1200)
		a.setStatusLine("Voice start failed: " + briefError(err.Error(), 80))
		return
	}

	a.stateMu.Lock()
	a.openHoldStop = nil
	a.openHoldKey = ""
	a.openHoldActionKey = actionKey
	a.openHoldProgress = 1
	a.voiceCapture = capture
	a.stateMu.Unlock()

	showVoiceRecordingNotification()
	a.setStatusLine("Recording voice for " + sessionListTitle(session) + "...")
	fyne.Do(func() {
		a.render()
		a.showVoiceRecordDialog(actionKey, session)
	})
}

func (a *App) showVoiceRecordDialog(actionKey string, session SessionResponse) {
	content := container.NewVBox(
		widget.NewLabelWithStyle("Recording voice", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Click Stop to transcribe, or Cancel to discard this recording."),
		widget.NewLabel("Session: "+sessionListTitle(session)),
	)

	confirm := dialog.NewCustomConfirm("Voice Recording", "Stop (Enter)", "Cancel (Esc)", content, func(ok bool) {
		if ok {
			a.finishVoiceClick(actionKey, false)
			return
		}
		a.finishVoiceClick(actionKey, true)
	}, a.window)
	confirm.SetOnClosed(func() {
		a.stateMu.Lock()
		if a.voiceRecordDialog == confirm {
			a.voiceRecordDialog = nil
		}
		a.stateMu.Unlock()
	})

	a.stateMu.Lock()
	a.voiceRecordDialog = confirm
	a.stateMu.Unlock()

	a.window.RequestFocus()
	confirm.Show()
}

func (a *App) finishVoiceClick(actionKey string, cancelled bool) {
	a.stateMu.Lock()
	if a.openHoldActionKey != actionKey {
		a.stateMu.Unlock()
		return
	}
	capture := a.voiceCapture
	a.voiceCapture = nil
	a.openHoldStop = nil
	a.openHoldKey = ""
	a.openHoldActionKey = ""
	a.openHoldProgress = 0
	recordDialog := a.voiceRecordDialog
	a.voiceRecordDialog = nil
	a.stateMu.Unlock()

	if recordDialog != nil {
		recordDialog.Hide()
	}
	fyne.Do(a.render)

	if capture == nil {
		closeVoiceNotification()
		return
	}
	if cancelled {
		_ = stopVoiceRecorder(capture.cmd)
		closeVoiceCaptureFile(capture.audioFile)
		showVoiceTimedNotification("uconsole voice", "录音已取消", 0, 800)
		a.setStatusLine("Voice input cancelled")
		return
	}
	if !a.startOpenSessionActionPending(actionKey) {
		return
	}
	go a.transcribeVoiceCapture(actionKey, capture.session, capture)
}

func (a *App) transcribeVoiceCapture(actionKey string, session SessionResponse, capture *voiceCapture) {
	showVoiceTranscribingNotification()
	a.setStatusLine("Transcribing voice for " + sessionListTitle(session) + "...")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	text, err := capture.StopAndTranscribe(ctx)
	if err != nil {
		a.finishOpenSessionActionPending(actionKey)
		if strings.Contains(strings.ToLower(err.Error()), "too short") {
			showVoiceTimedNotification("uconsole voice", "录音太短，已取消", 0, 800)
			a.setStatusLine("Voice input cancelled")
			return
		}
		showVoiceTimedNotification("uconsole voice", "识别失败", 0, 1200)
		a.setStatusLine("Voice failed: " + briefError(err.Error(), 80))
		return
	}

	closeVoiceNotification()
	fyne.Do(func() {
		a.showVoiceDialog(session, actionKey, text)
	})
}

func (a *App) showVoiceDialog(session SessionResponse, actionKey string, text string) {
	entry := widget.NewEntry()
	entry.SetText(strings.TrimSpace(text))

	items := []*widget.FormItem{
		{Text: "Text", Widget: entry},
	}

	formDialog := dialog.NewForm("Voice Command", "Send (Enter)", "Cancel (Esc)", items, func(ok bool) {
		if !ok {
			a.finishOpenSessionActionPending(actionKey)
			return
		}

		value := strings.TrimSpace(entry.Text)
		if value == "" {
			a.finishOpenSessionActionPending(actionKey)
			a.setStatusLine("Voice input cancelled")
			return
		}

		go a.sendVoiceDialogText(session, actionKey, value)
	}, a.window)
	formDialog.Resize(fyne.NewSize(760, 180))
	formDialog.SetOnClosed(func() {
		a.stateMu.Lock()
		if a.voiceDialog == formDialog {
			a.voiceDialog = nil
			a.voiceDialogEntry = nil
			a.voiceDialogActionKey = ""
			a.voiceDialogSession = SessionResponse{}
		}
		a.stateMu.Unlock()
	})

	a.stateMu.Lock()
	a.voiceDialog = formDialog
	a.voiceDialogEntry = entry
	a.voiceDialogActionKey = actionKey
	a.voiceDialogSession = session
	a.stateMu.Unlock()

	a.window.RequestFocus()
	formDialog.Show()
}

func (a *App) sendVoiceDialogText(session SessionResponse, actionKey string, text string) {
	defer a.finishOpenSessionActionPending(actionKey)

	client := a.clientForServer(session.ServerID)
	if client == nil {
		a.setStatusLine("Selected server is unavailable")
		return
	}

	a.setStatusLine("Sending voice text to " + sessionListTitle(session) + "...")
	if err := client.SendSessionText(context.Background(), session, text); err != nil {
		a.setStatusLine(err.Error())
		return
	}
	a.setStatusLine("Voice text sent")
	_ = a.refreshNow(context.Background(), true)
}

const voiceNotifyID = 82421

func showVoiceRecordingNotification() {
	showVoiceNotification("uconsole voice", "录音中...", 20, 0)
}

func showVoiceTranscribingNotification() {
	showVoiceNotification("uconsole voice", "识别中...", 65, 0)
}

func showVoiceTimedNotification(summary, body string, value int, timeoutMS int) {
	showVoiceNotification(summary, body, value, timeoutMS)
}

func closeVoiceNotification() {
	if _, err := exec.LookPath("dunstify"); err == nil {
		cmd := exec.Command("dunstify", "-C", strconv.Itoa(voiceNotifyID))
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		_ = cmd.Run()
	}
}

func showVoiceNotification(summary, body string, value int, timeoutMS int) {
	if timeoutMS <= 0 {
		timeoutMS = 0
	}
	if _, err := exec.LookPath("dunstify"); err == nil {
		args := []string{
			"-a", "uconsole-voice",
			"-r", strconv.Itoa(voiceNotifyID),
			"-u", "low",
			"-t", strconv.Itoa(timeoutMS),
		}
		if value > 0 {
			args = append(args, "-h", "int:value:"+strconv.Itoa(value))
		}
		args = append(args, summary, body)
		cmd := exec.Command("dunstify", args...)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		_ = cmd.Run()
		return
	}
	if _, err := exec.LookPath("notify-send"); err == nil {
		cmd := exec.Command("notify-send", summary, body)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		_ = cmd.Run()
	}
}

func closeVoiceCaptureFile(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.Remove(path)
}
