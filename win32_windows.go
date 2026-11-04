package main

import (
	"math"
	"path/filepath"
	"syscall"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll") // Windows KnownDLL

func systemDLL(name string) *syscall.LazyDLL {
	var path [260]uint16
	n, _, err := kernel.NewProc("GetSystemDirectoryW").Call(uintptr(unsafe.Pointer(&path[0])), uintptr(len(path)))
	if n == 0 || n >= uintptr(len(path)) {
		panic(err)
	}
	return syscall.NewLazyDLL(filepath.Join(syscall.UTF16ToString(path[:]), name))
}

var user = systemDLL("user32.dll")
var openWithShell = systemDLL("shell32.dll").NewProc("ShellExecuteW")
var linkCursor, _, _ = user.NewProc("LoadCursorW").Call(0, 32649) // IDC_HAND
var gdi = systemDLL("gdi32.dll")
var dwm = systemDLL("dwmapi.dll")
var msimg = systemDLL("msimg32.dll")
var comdlg = systemDLL("comdlg32.dll")

var (
	registerClass = user.NewProc("RegisterClassExW")
	createWindow  = user.NewProc("CreateWindowExW")
	defaultProc   = user.NewProc("DefWindowProcW")
	showWindow    = user.NewProc("ShowWindow")
	setWindowPos  = user.NewProc("SetWindowPos")
	getWindowRect = user.NewProc("GetWindowRect")
	getClient     = user.NewProc("GetClientRect")
	invalidate    = user.NewProc("InvalidateRect")
	beginPaint    = user.NewProc("BeginPaint")
	endPaint      = user.NewProc("EndPaint")
	getDC         = user.NewProc("GetDC")
	releaseDC     = user.NewProc("ReleaseDC")
	newDC         = gdi.NewProc("CreateCompatibleDC")
	deleteDC      = gdi.NewProc("DeleteDC")
	selectObj     = gdi.NewProc("SelectObject")
	deleteObj     = gdi.NewProc("DeleteObject")
	makeBitmap    = gdi.NewProc("CreateDIBSection")
	stockObj      = gdi.NewProc("GetStockObject")
	brushColor    = gdi.NewProc("SetDCBrushColor")
	penColor      = gdi.NewProc("SetDCPenColor")
	drawRect      = gdi.NewProc("Rectangle")
	roundRect     = gdi.NewProc("RoundRect")
	drawEllipse   = gdi.NewProc("Ellipse")
	fillRect      = user.NewProc("FillRect")
	moveTo        = gdi.NewProc("MoveToEx")
	lineTo        = gdi.NewProc("LineTo")
	textColor     = gdi.NewProc("SetTextColor")
	drawText      = user.NewProc("DrawTextW")
	alphaBlend    = msimg.NewProc("AlphaBlend")
	bitBlt        = gdi.NewProc("BitBlt")
	createFont    = gdi.NewProc("CreateFontW")
	updateLayer   = user.NewProc("UpdateLayeredWindow")
)

func str(s string) *uint16 { v, _ := syscall.UTF16PtrFromString(s); return v }
func u(v int) uintptr      { return uintptr(v) }

type point struct{ X, Y int32 }
type rect struct{ L, T, R, B int32 }

func (r rect) width() int  { return int(r.R - r.L) }
func (r rect) height() int { return int(r.B - r.T) }

type message struct {
	Window  uintptr
	ID      uint32
	W, L    uintptr
	Time    uint32
	Point   point
	Private uint32
}
type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type paintStruct struct {
	DC              uintptr
	Erase           int32
	Area            rect
	Restore, Update int32
	Reserved        [32]byte
}
type bitmapHeader struct {
	Size                   uint32
	Width, Height          int32
	Planes, BitCount       uint16
	Compression, SizeImage uint32
	XPels, YPels           int32
	Used, Important        uint32
}

// A top-down, 32-bit DIB. Win32 stores little-endian BGRA and expects
// premultiplied channels for both AlphaBlend and UpdateLayeredWindow.
type surface struct {
	dc, bmp, previous uintptr
	pixels            []uint32
	w, h              int
}

func newSurface(w, h int) *surface {
	if w <= 0 || h <= 0 {
		return nil
	}
	dc, _, _ := newDC.Call(0)
	if dc == 0 {
		return nil
	}
	header := bitmapHeader{Size: 40, Width: int32(w), Height: int32(-h), Planes: 1, BitCount: 32}
	var data *uint32
	bmp, _, _ := makeBitmap.Call(dc, uintptr(unsafe.Pointer(&header)), 0, uintptr(unsafe.Pointer(&data)), 0, 0)
	if bmp == 0 {
		deleteDC.Call(dc)
		return nil
	}
	prev, _, _ := selectObj.Call(dc, bmp)
	return &surface{dc, bmp, prev, unsafe.Slice(data, w*h), w, h}
}
func (s *surface) close() {
	if s != nil {
		selectObj.Call(s.dc, s.previous)
		deleteObj.Call(s.bmp)
		deleteDC.Call(s.dc)
	}
}

func colorRef(c uint32) uintptr { return uintptr(c&0xff00 | c>>16&255 | (c&255)<<16) }

var dcBrush, _, _ = stockObj.Call(18)
var dcPen, _, _ = stockObj.Call(19)

type canvas struct {
	dc    uintptr
	scale float64
}

func (c canvas) p(v int) int { return int(math.Round(float64(v) * c.scale)) }
func (c canvas) box(x, y, w, h int) rect {
	return rect{int32(c.p(x)), int32(c.p(y)), int32(c.p(x + w)), int32(c.p(y + h))}
}
func (c canvas) fill(x, y, w, h int, col uint32) {
	r := c.box(x, y, w, h)
	brushColor.Call(c.dc, colorRef(col))
	fillRect.Call(c.dc, uintptr(unsafe.Pointer(&r)), dcBrush)
}
func (c canvas) shape(x, y, w, h, radius int, fill, border uint32, ellipse bool) {
	r := c.box(x, y, w, h)
	selectObj.Call(c.dc, dcBrush)
	selectObj.Call(c.dc, dcPen)
	brushColor.Call(c.dc, colorRef(fill))
	penColor.Call(c.dc, colorRef(border))
	if ellipse {
		drawEllipse.Call(c.dc, uintptr(r.L), uintptr(r.T), uintptr(r.R), uintptr(r.B))
		return
	}
	roundRect.Call(c.dc, uintptr(r.L), uintptr(r.T), uintptr(r.R), uintptr(r.B), u(c.p(radius*2)), u(c.p(radius*2)))
}
func (c canvas) card(x, y, w, h, r int, fill, border uint32) {
	c.shape(x, y, w, h, r, fill, border, false)
}
func (c canvas) circle(x, y, d int, fill, border uint32) { c.shape(x, y, d, d, 0, fill, border, true) }
func (c canvas) line(x, y, xx, yy int, col uint32) {
	selectObj.Call(c.dc, dcPen)
	penColor.Call(c.dc, colorRef(col))
	moveTo.Call(c.dc, u(c.p(x)), u(c.p(y)), 0)
	lineTo.Call(c.dc, u(c.p(xx)), u(c.p(yy)))
}

type fontKey struct {
	height, weight int
	mono           bool
}

var fonts = map[fontKey]uintptr{}

func (c canvas) label(x, y, w, h int, s string, size, weight int, col uint32, flags int, mono bool) {
	if s == "" {
		return
	}
	key := fontKey{max(1, c.p(size)), weight, mono}
	f := fonts[key]
	if f == 0 {
		family := "Segoe UI"
		if mono {
			family = "Consolas"
		}
		f, _, _ = createFont.Call(u(-key.height), 0, 0, 0, u(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(str(family))))
		fonts[key] = f
	}
	selectObj.Call(c.dc, f)
	textColor.Call(c.dc, colorRef(col))
	r := c.box(x, y, w, h)
	drawText.Call(c.dc, uintptr(unsafe.Pointer(str(s))), u(-1), uintptr(unsafe.Pointer(&r)), u(0x20|0x04|0x800|0x8000|flags))
}
func (c canvas) text(x, y, w, h int, s string, size, weight int, col uint32) {
	c.label(x, y, w, h, s, size, weight, col, 0, false)
}
func (c canvas) mono(x, y, w, h int, s string, size int, col uint32) {
	c.label(x, y, w, h, s, size, 500, col, 0, true)
}
func (c canvas) reticle(cx, cy, kind int, r reticle, zoom float64) {
	s := makeReticle(kind, r, zoom*c.scale)
	d := newSurface(s.side, s.side)
	if d == nil {
		return
	}
	defer d.close()
	copy(d.pixels, s.pixels)
	alphaBlend.Call(c.dc, u(c.p(cx)-s.side/2), u(c.p(cy)-s.side/2), u(s.side), u(s.side), d.dc, 0, 0, u(s.side), u(s.side), 0x01ff0000)
}

type monitorInfo struct {
	Size         uint32
	Bounds, Work rect
	Flags        uint32
	Device       [32]uint16
}
type display struct {
	name         string
	bounds, work rect
	primary      bool
}

var displays []display
var enumMonitorProc = syscall.NewCallback(func(m, dc, rc, param uintptr) uintptr {
	var info monitorInfo
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := user.NewProc("GetMonitorInfoW").Call(m, uintptr(unsafe.Pointer(&info))); ok != 0 {
		displays = append(displays, display{syscall.UTF16ToString(info.Device[:]), info.Bounds, info.Work, info.Flags&1 != 0})
	}
	return 1
})

func refreshDisplays() {
	displays = nil
	user.NewProc("EnumDisplayMonitors").Call(0, 0, enumMonitorProc, 0)
	if len(displays) == 0 {
		w, _, _ := user.NewProc("GetSystemMetrics").Call(0)
		h, _, _ := user.NewProc("GetSystemMetrics").Call(1)
		r := rect{0, 0, int32(w), int32(h)}
		displays = []display{{"", r, r, true}}
	}
}
func selectedDisplay() int {
	for i, d := range displays {
		if d.name == state.Monitor {
			return i
		}
	}
	for i, d := range displays {
		if d.primary {
			return i
		}
	}
	return 0
}
