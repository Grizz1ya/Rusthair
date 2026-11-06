package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	editorClass    = "Rusthair.Editor.1"
	layerClass     = "Rusthair.Reticle.1"
	mainStyle      = 0x00ca0000 // caption, system menu and minimize button
	wmRefresh      = 0x8001
	wmRestore      = 0x8002
	wmUpdateResult = 0x8003
	wmTray         = 0x8004
	saveTimer      = 1
	updateTimer    = 2
	trayRetryTimer = 3
)

var state settings
var mainWindow, overlayWindow, instance uintptr
var dirty, trackingMouse bool
var lastSprite *surface
var spriteKind = -1
var spriteStyle reticle
var customColors [16]uint32
var overlayFailed bool
var appProc = syscall.NewCallback(editorProcedure)
var layerProc = syscall.NewCallback(overlayProcedure)

func repaint() {
	if mainWindow != 0 {
		invalidate.Call(mainWindow, 0, 0)
	}
}
func flush() {
	if !dirty {
		return
	}
	if err := saveSettings(state); err != nil {
		statusText = "Не удалось сохранить настройки"
	} else {
		dirty = false
		statusText = "Настройки сохранены"
	}
}
func changed() {
	dirty = true
	statusText = "Сохранение…"
	user.NewProc("SetTimer").Call(mainWindow, saveTimer, 350, 0)
	refreshOverlay()
	updateTray()
	repaint()
}

func refreshOverlay() {
	if overlayWindow == 0 {
		return
	}
	if !state.Enabled {
		showWindow.Call(overlayWindow, 0)
		return
	}
	b := displays[selectedDisplay()].bounds
	cx, cy := int(b.L)+b.width()/2+state.OffsetX, int(b.T)+b.height()/2+state.OffsetY
	fg, _, _ := user.NewProc("GetForegroundWindow").Call()
	root, _, _ := user.NewProc("GetAncestor").Call(fg, 3)
	var editor rect
	getWindowRect.Call(mainWindow, uintptr(unsafe.Pointer(&editor)))
	minimized, _, _ := user.NewProc("IsIconic").Call(mainWindow)
	visible, _, _ := user.NewProc("IsWindowVisible").Call(mainWindow)
	if root == mainWindow && visible != 0 && minimized == 0 && cx >= int(editor.L) && cx < int(editor.R) && cy >= int(editor.T) && cy < int(editor.B) {
		showWindow.Call(overlayWindow, 0)
		return
	}
	style := state.Styles[state.Selected]
	if lastSprite == nil || spriteKind != state.Selected || spriteStyle != style {
		s := makeReticle(state.Selected, style, 1)
		surf := newSurface(s.side, s.side)
		if surf == nil {
			overlayError()
			return
		}
		copy(surf.pixels, s.pixels)
		lastSprite.close()
		lastSprite = surf
		spriteKind = state.Selected
		spriteStyle = style
	}
	dst := point{int32(cx - lastSprite.w/2), int32(cy - lastSprite.h/2)}
	size := point{int32(lastSprite.w), int32(lastSprite.h)}
	src := point{}
	blend := uint32(0x01ff0000)
	ok, _, _ := updateLayer.Call(overlayWindow, 0, uintptr(unsafe.Pointer(&dst)), uintptr(unsafe.Pointer(&size)), lastSprite.dc, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), 2)
	if ok == 0 {
		overlayError()
		return
	}
	overlayFailed = false
	setWindowPos.Call(overlayWindow, ^uintptr(0), 0, 0, 0, 0, 0x53) // topmost, show, no move/size/activation
}
func overlayError() { overlayFailed = true; showWindow.Call(overlayWindow, 0); repaint() }
func overlayProcedure(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case 0x84:
		return ^uintptr(0) // HTTRANSPARENT
	case 0x21:
		return 3 // MA_NOACTIVATE
	}
	v, _, _ := defaultProc.Call(hwnd, msg, wp, lp)
	return v
}

type colorDialog struct {
	Size            uint32
	Owner, Instance uintptr
	RGB             uint32
	Custom          *uint32
	Flags           uint32
	Data, Hook      uintptr
	Template        *uint16
}

func pickColor(r *reticle) bool {
	d := colorDialog{Owner: mainWindow, RGB: uint32(colorRef(r.Color)), Custom: &customColors[0], Flags: 3}
	d.Size = uint32(unsafe.Sizeof(d))
	ok, _, _ := comdlg.NewProc("ChooseColorW").Call(uintptr(unsafe.Pointer(&d)))
	if ok != 0 {
		r.Color = uint32(colorRef(d.RGB))
		return true
	}
	return false
}

type mouseTracking struct {
	Size, Flags uint32
	Window      uintptr
	HoverTime   uint32
}

func mousePosition(lp uintptr) (int, int) {
	return int(math.Floor(float64(int(int16(lp))-originX) / displayScale)), int(math.Floor(float64(int(int16(lp>>16))-originY) / displayScale))
}
func restoreMenu() {
	showWindow.Call(mainWindow, 9)
	user.NewProc("SetForegroundWindow").Call(mainWindow)
}
func focusNext(backward bool) {
	if len(controls) == 0 {
		return
	}
	i := -1
	for j, r := range controls {
		if r.id == focused {
			i = j
			break
		}
	}
	if backward {
		if i < 0 {
			i = 0
		}
		i = (i - 1 + len(controls)) % len(controls)
	} else {
		i = (i + 1) % len(controls)
	}
	focused = controls[i].id
	keyboardFocus = true
	repaint()
}
func editorProcedure(hwnd, msg, wp, lp uintptr) uintptr {
	if taskbarCreated != 0 && msg == taskbarCreated {
		trayPresent = false
		if !addTray() {
			visible, _, _ := user.NewProc("IsWindowVisible").Call(hwnd)
			if visible == 0 {
				showWindow.Call(hwnd, 7)
			}
		}
		return 0
	}
	switch msg {
	case wmTray:
		trayEvent(wp, lp)
		return 0
	case wmUpdateResult:
		receiveUpdate()
		return 0
	case 0x0f:
		paintEditor(hwnd)
		return 0
	case 0x14:
		return 1
	case 0x03, 0x05, 0x06: // move, size, activation
		if msg == 0x06 && wp&0xffff != 0 && mainWindow != 0 {
			refreshStartup()
			repaint()
		}
		if mainWindow != 0 {
			user.NewProc("PostMessageW").Call(hwnd, wmRefresh, 0, 0)
		}
	case wmRefresh:
		refreshOverlay()
		return 0
	case wmRestore:
		restoreMenu()
		return 0
	case 0x7e:
		refreshDisplays()
		refreshOverlay()
		repaint()
		return 0
	case 0x02e0:
		var r rect
		// WM_DPICHANGED supplies a system-owned RECT valid for this callback.
		kernel.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&r)), lp, unsafe.Sizeof(r))
		w, h := editorDimensions(int(wp&0xffff), workAtRect(r))
		setWindowPos.Call(hwnd, 0, uintptr(r.L), uintptr(r.T), u(w), u(h), 0x14)
		repaint()
		return 0
	case 0x200:
		x, y := mousePosition(lp)
		if drag != "" {
			slideTo(drag, x)
			return 0
		}
		if !trackingMouse {
			t := mouseTracking{Flags: 2, Window: hwnd}
			t.Size = uint32(unsafe.Sizeof(t))
			ok, _, _ := user.NewProc("TrackMouseEvent").Call(uintptr(unsafe.Pointer(&t)))
			trackingMouse = ok != 0
		}
		id := idAt(x, y)
		if id == "creator" {
			user.NewProc("SetCursor").Call(linkCursor)
		}
		if id != hovered {
			hovered = id
			repaint()
		}
		return 0
	case 0x2a3:
		hovered = ""
		trackingMouse = false
		repaint()
		return 0
	case 0x201:
		x, y := mousePosition(lp)
		focused = idAt(x, y)
		keyboardFocus = false
		if slideTo(focused, x) {
			drag = focused
			user.NewProc("SetCapture").Call(hwnd)
		}
		repaint()
		return 0
	case 0x202:
		x, y := mousePosition(lp)
		if drag != "" {
			slideTo(drag, x)
			drag = ""
			user.NewProc("ReleaseCapture").Call()
			return 0
		}
		if id := idAt(x, y); id != "" && id == focused {
			activate(id)
		}
		return 0
	case 0x215:
		drag = ""
		return 0
	case 0x100:
		switch wp {
		case 9:
			shift, _, _ := user.NewProc("GetKeyState").Call(0x10)
			focusNext(shift&0x8000 != 0)
			return 0
		case 32, 13:
			if lp&(1<<30) == 0 {
				activate(focused)
			}
			return 0
		case 0x25, 0x28:
			if stepSlider(focused, -1) {
				return 0
			}
		case 0x27, 0x26:
			if stepSlider(focused, 1) {
				return 0
			}
		case 27:
			showWindow.Call(hwnd, 6)
			return 0
		}
	case 0x312:
		if wp == 1 {
			activate("power")
		} else if wp == 2 {
			fg, _, _ := user.NewProc("GetForegroundWindow").Call()
			minimized, _, _ := user.NewProc("IsIconic").Call(hwnd)
			if fg == hwnd && minimized == 0 {
				showWindow.Call(hwnd, 6)
			} else {
				restoreMenu()
			}
		}
		return 0
	case 0x113:
		switch wp {
		case saveTimer:
			user.NewProc("KillTimer").Call(hwnd, saveTimer)
			flush()
			repaint()
		case updateTimer:
			checkForUpdates()
		case trayRetryTimer:
			addTray()
		}
		return 0
	case 0x11:
		flush()
		return 1
	case 0x10:
		hideToTray()
		return 0
	case 0x16: // WM_ENDSESSION, after Windows confirms sign-out/shutdown
		if wp != 0 {
			user.NewProc("DestroyWindow").Call(hwnd)
		}
		return 0
	case 2:
		flush()
		removeTray()
		user.NewProc("KillTimer").Call(hwnd, updateTimer)
		if overlayWindow != 0 {
			user.NewProc("DestroyWindow").Call(overlayWindow)
			overlayWindow = 0
		}
		user.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	v, _, _ := defaultProc.Call(hwnd, msg, wp, lp)
	return v
}

func register(name string, callback, icon uintptr) error {
	cursor, _, _ := user.NewProc("LoadCursorW").Call(0, 32512)
	w := windowClass{Proc: callback, Instance: instance, Icon: icon, SmallIcon: icon, Cursor: cursor, Name: str(name)}
	w.Size = uint32(unsafe.Sizeof(w))
	v, _, err := registerClass.Call(uintptr(unsafe.Pointer(&w)))
	if v == 0 {
		return err
	}
	return nil
}
func workAtRect(r rect) rect {
	m, _, _ := user.NewProc("MonitorFromRect").Call(uintptr(unsafe.Pointer(&r)), 2)
	var info monitorInfo
	info.Size = uint32(unsafe.Sizeof(info))
	v, _, _ := user.NewProc("GetMonitorInfoW").Call(m, uintptr(unsafe.Pointer(&info)))
	if v != 0 {
		return info.Work
	}
	return displays[selectedDisplay()].work
}
func editorDimensions(dpi int, work rect) (int, int) {
	var frame rect
	user.NewProc("AdjustWindowRectExForDpi").Call(uintptr(unsafe.Pointer(&frame)), mainStyle, 0, 0, u(dpi))
	scale := math.Min(float64(dpi)/96, math.Min(float64(work.width()-frame.width()-12)/editorW, float64(work.height()-frame.height()-12)/editorH))
	return int(math.Round(editorW*scale)) + frame.width(), int(math.Round(editorH*scale)) + frame.height()
}

type shortcut struct {
	key   uintptr
	label string
}

func registerShortcut(id uintptr, choices ...shortcut) string {
	for _, choice := range choices {
		ok, _, _ := user.NewProc("RegisterHotKey").Call(mainWindow, id, 0x4003, choice.key)
		if ok != 0 {
			return "Ctrl+Alt+" + choice.label
		}
	}
	return "занято"
}

func run() error {
	var cancel context.CancelFunc
	updateContext, cancel = context.WithCancel(context.Background())
	defer cancel()
	atLogon := len(os.Args) == 2 && os.Args[1] == startupFlag
	user.NewProc("SetProcessDpiAwarenessContext").Call(^uintptr(3)) // PER_MONITOR_AWARE_V2
	mutex, _, mutexErr := kernel.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(str("Local\\Rusthair.Editor.1"))))
	if mutex == 0 {
		return mutexErr
	}
	defer kernel.NewProc("CloseHandle").Call(mutex)
	if errors.Is(mutexErr, syscall.Errno(183)) {
		if atLogon {
			return nil
		}
		h, _, _ := user.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(str(editorClass))), 0)
		if h != 0 {
			var pid uint32
			user.NewProc("GetWindowThreadProcessId").Call(h, uintptr(unsafe.Pointer(&pid)))
			user.NewProc("AllowSetForegroundWindow").Call(uintptr(pid))
			user.NewProc("PostMessageW").Call(h, wmRestore, 0, 0)
		}
		return nil
	}
	var err error
	state, err = loadSettings()
	if err != nil {
		statusText = "Не удалось прочитать настройки"
	}
	refreshStartup()
	refreshDisplays()
	instance, _, _ = kernel.NewProc("GetModuleHandleW").Call(0)
	icon, _, _ := user.NewProc("LoadIconW").Call(instance, 1)
	if err = register(editorClass, appProc, icon); err != nil {
		return err
	}
	if err = register(layerClass, layerProc, 0); err != nil {
		return err
	}
	var primary display
	for _, d := range displays {
		if d.primary {
			primary = d
			break
		}
	}
	if primary.work.width() == 0 {
		primary = displays[0]
	}
	dpi, _, _ := user.NewProc("GetDpiForSystem").Call()
	w, h := editorDimensions(int(dpi), primary.work)
	x := int(primary.work.L) + (primary.work.width()-w)/2
	y := int(primary.work.T) + (primary.work.height()-h)/2
	mainWindow, _, err = createWindow.Call(0, uintptr(unsafe.Pointer(str(editorClass))), uintptr(unsafe.Pointer(str("Rusthair — Мастерская прицелов"))), mainStyle, u(x), u(y), u(w), u(h), 0, 0, instance, 0)
	if mainWindow == 0 {
		return fmt.Errorf("окно редактора: %w", err)
	}
	// A regular compositor window, never an input target or an Alt+Tab entry.
	const layerStyles = 0x80000 | 0x20 | 0x8000000 | 0x80 | 0x8
	overlayWindow, _, err = createWindow.Call(layerStyles, uintptr(unsafe.Pointer(str(layerClass))), uintptr(unsafe.Pointer(str("Rusthair Overlay"))), 0x80000000, 0, 0, 1, 1, 0, 0, instance, 0)
	if overlayWindow == 0 {
		return fmt.Errorf("окно прицела: %w", err)
	}
	dark := int32(1)
	dwm.NewProc("DwmSetWindowAttribute").Call(mainWindow, 20, uintptr(unsafe.Pointer(&dark)), 4)
	caption := uint32(colorRef(bg))
	dwm.NewProc("DwmSetWindowAttribute").Call(mainWindow, 35, uintptr(unsafe.Pointer(&caption)), 4)
	for i := range customColors {
		customColors[i] = uint32(colorRef(swatches[i%len(swatches)]))
	}
	hotOverlay = registerShortcut(1, shortcut{'X', "X"}, shortcut{0x78, "F9"}, shortcut{0x76, "F7"})
	hotMenu = registerShortcut(2, shortcut{'M', "M"}, shortcut{0x79, "F10"}, shortcut{0x77, "F8"})
	trayIcon = icon
	addTray()
	if atLogon {
		showWindow.Call(mainWindow, 7) // SW_SHOWMINNOACTIVE
	} else {
		showWindow.Call(mainWindow, 5)
	}
	user.NewProc("UpdateWindow").Call(mainWindow)
	refreshOverlay()
	checkForUpdates()
	user.NewProc("SetTimer").Call(mainWindow, updateTimer, uintptr(updateInterval.Milliseconds()), 0)
	var m message
	for {
		got, _, e := user.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(got) == -1 {
			return e
		}
		if got == 0 {
			break
		}
		user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
	lastSprite.close()
	scenePhoto.bitmap.close()
	creatorPhoto.bitmap.close()
	for _, f := range fonts {
		deleteObj.Call(f)
	}
	return nil
}
func main() {
	runtime.LockOSThread()
	if err := run(); err != nil {
		user.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(str(err.Error()))), uintptr(unsafe.Pointer(str("Rusthair"))), 0x10)
	}
}
