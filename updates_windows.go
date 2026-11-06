package main

import (
	"context"
	"net/http"
	"time"
	"unsafe"
)

type updateResult struct {
	release releaseInfo
	err     error
}
type updateState struct {
	checking, checked bool
	release           releaseInfo
	err               error
}

var updates updateState
var updateResults = make(chan updateResult, 1)
var updateContext context.Context

func checkForUpdates() {
	if updates.checking || updateContext == nil {
		return
	}
	updates.checking = true
	repaint()
	hwnd := mainWindow
	ctx := updateContext
	go func() {
		release, err := fetchLatestRelease(ctx, &http.Client{Timeout: 12 * time.Second}, latestReleaseAPI)
		select {
		case <-ctx.Done():
			return
		case updateResults <- updateResult{release, err}:
			user.NewProc("PostMessageW").Call(hwnd, wmUpdateResult, 0, 0)
		}
	}()
}

func updateAvailable() bool {
	newer, err := newerVersion(updates.release.Tag, appVersion)
	return err == nil && newer
}

func receiveUpdate() {
	select {
	case result := <-updateResults:
		previous := updates.release.Tag
		updates.checking = false
		updates.checked = true
		updates.err = result.err
		if result.err == nil {
			updates.release = result.release
		}
		updateTray()
		if updateAvailable() && updates.release.Tag != previous {
			announceUpdate()
		}
		repaint()
	default:
	}
}

func updateLabel() string {
	if updateAvailable() {
		return "Доступна " + updates.release.Tag + " · Открыть релиз"
	}
	if updates.checking {
		return "Проверка обновлений…"
	}
	if updates.err != nil {
		return "Не удалось проверить · Повторить"
	}
	if updates.checked {
		return "Версия актуальна · Проверить"
	}
	return "Проверить обновления"
}

func openRelease() {
	if !updateAvailable() {
		return
	}
	result, _, _ := openWithShell.Call(mainWindow, uintptr(unsafe.Pointer(str("open"))),
		uintptr(unsafe.Pointer(str(updates.release.pageURL()))), 0, 0, 1)
	if result <= 32 {
		statusText = "Не удалось открыть страницу релиза"
		repaint()
	}
}
