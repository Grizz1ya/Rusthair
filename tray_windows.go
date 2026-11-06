package main

import (
	"syscall"
	"unsafe"
)

const (
	trayOpen   = 1001
	trayToggle = 1002
	trayUpdate = 1003
	trayExit   = 1004
)

type notifyIconData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                [16]byte
	BalloonIcon         uintptr
}

var shellNotify = systemDLL("shell32.dll").NewProc("Shell_NotifyIconW")
var taskbarCreated, _, _ = user.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(str("TaskbarCreated"))))
var trayIcon uintptr
var trayPresent, trayVersion4 bool

func trayData() notifyIconData {
	d := notifyIconData{Window: mainWindow, ID: 1, Icon: trayIcon, Callback: wmTray}
	d.Size = uint32(unsafe.Sizeof(d))
	tip := "Rusthair " + appVersion + " · прицел выключен"
	if state.Enabled {
		tip = "Rusthair " + appVersion + " · прицел включён"
	}
	if updateAvailable() {
		tip += " · доступна " + updates.release.Tag
	}
	encoded, _ := syscall.UTF16FromString(tip)
	copy(d.Tip[:len(d.Tip)-1], encoded)
	return d
}

func addTray() bool {
	d := trayData()
	d.Flags = 0x87 // NIF_MESSAGE | NIF_ICON | NIF_TIP | NIF_SHOWTIP
	ok, _, _ := shellNotify.Call(0, uintptr(unsafe.Pointer(&d)))
	trayPresent = ok != 0
	if trayPresent {
		d.Version = 4
		v, _, _ := shellNotify.Call(4, uintptr(unsafe.Pointer(&d)))
		trayVersion4 = v != 0
		user.NewProc("KillTimer").Call(mainWindow, trayRetryTimer)
	} else {
		user.NewProc("SetTimer").Call(mainWindow, trayRetryTimer, 5000, 0)
	}
	return trayPresent
}

func updateTray() {
	if !trayPresent {
		return
	}
	d := trayData()
	d.Flags = 4
	shellNotify.Call(1, uintptr(unsafe.Pointer(&d)))
}

func removeTray() {
	user.NewProc("KillTimer").Call(mainWindow, trayRetryTimer)
	if trayPresent {
		d := trayData()
		shellNotify.Call(2, uintptr(unsafe.Pointer(&d)))
		trayPresent = false
	}
}

func hideToTray() {
	flush()
	if !trayPresent && !addTray() {
		statusText = "Значок трея недоступен. Окно свёрнуто на панель задач."
		showWindow.Call(mainWindow, 7)
	} else {
		showWindow.Call(mainWindow, 0)
	}
	refreshOverlay()
}

func announceUpdate() {
	if !trayPresent {
		return
	}
	d := trayData()
	d.Flags = 0x10
	title, _ := syscall.UTF16FromString("Доступна новая версия Rusthair")
	body, _ := syscall.UTF16FromString(updates.release.Tag + " — нажмите, чтобы открыть релиз на GitHub.")
	copy(d.InfoTitle[:], title)
	copy(d.Info[:], body)
	d.InfoFlags = 0x81 // NIIF_INFO | NIIF_RESPECT_QUIET_TIME
	shellNotify.Call(1, uintptr(unsafe.Pointer(&d)))
}

func trayMenu(pos point) {
	menu, _, _ := user.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer user.NewProc("DestroyMenu").Call(menu)
	appendItem := func(id int, title string, flags uintptr) {
		user.NewProc("AppendMenuW").Call(menu, flags, u(id), uintptr(unsafe.Pointer(str(title))))
	}
	appendItem(trayOpen, "Открыть Rusthair", 0)
	var checked uintptr
	if state.Enabled {
		checked = 8
	}
	appendItem(trayToggle, "Показывать прицел", checked)
	label := "Проверить обновления"
	if updateAvailable() {
		label = "Доступна " + updates.release.Tag + " — открыть релиз"
	}
	if updates.checking && !updateAvailable() {
		label = "Проверка обновлений…"
	}
	appendItem(trayUpdate, label, 0)
	appendItem(0, "", 0x800)
	appendItem(trayExit, "Выход", 0)
	user.NewProc("SetMenuDefaultItem").Call(menu, trayOpen, 0)
	user.NewProc("SetForegroundWindow").Call(mainWindow)
	selected, _, _ := user.NewProc("TrackPopupMenu").Call(menu, 0x102, uintptr(pos.X), uintptr(pos.Y), 0, mainWindow, 0)
	user.NewProc("PostMessageW").Call(mainWindow, 0, 0, 0) // dismiss correctly on the next click outside
	switch selected {
	case trayOpen:
		restoreMenu()
	case trayToggle:
		activate("power")
	case trayUpdate:
		if updateAvailable() {
			openRelease()
		} else {
			checkForUpdates()
		}
	case trayExit:
		user.NewProc("DestroyWindow").Call(mainWindow)
	default:
		d := trayData()
		shellNotify.Call(3, uintptr(unsafe.Pointer(&d)))
	}
}

func trayEvent(wp, lp uintptr) {
	event := lp
	var pos point
	if trayVersion4 {
		if lp>>16 != 1 {
			return
		}
		event = lp & 0xffff
		pos = point{int32(int16(wp)), int32(int16(wp >> 16))}
	} else {
		if wp != 1 {
			return
		}
		user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pos)))
	}
	switch event {
	case 0x400, 0x401, 0x202:
		restoreMenu() // NIN_SELECT, NIN_KEYSELECT, legacy click
	case 0x7b, 0x205:
		trayMenu(pos)
	case 0x405:
		openRelease() // NIN_BALLOONUSERCLICK
	}
}
