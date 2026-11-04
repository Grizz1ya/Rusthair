package main

import (
	"fmt"
	"math"
	"strings"
	"unsafe"
)

const (
	editorW   = 960
	editorH   = 628
	bg        = 0x14191e
	panel     = 0x1c2228
	raised    = 0x252c33
	border    = 0x343c44
	ink       = 0xf3f1eb
	muted     = 0xb0b6ba
	dim       = 0x89949c
	amber     = 0xffb876
	amberTint = 0x38302a
)

type hit struct {
	id         string
	x, y, w, h int
}

var controls []hit
var hovered, focused, drag string
var keyboardFocus, displayTab bool
var scenePhoto, creatorPhoto photoCache
var displayScale = 1.0
var originX, originY int
var statusText string
var hotOverlay, hotMenu string

func active(r hit, x, y int) bool   { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }
func add(id string, x, y, w, h int) { controls = append(controls, hit{id, x, y, w, h}) }
func idAt(x, y int) string {
	for i := len(controls) - 1; i >= 0; i-- {
		if active(controls[i], x, y) {
			return controls[i].id
		}
	}
	return ""
}
func hi(id string, normal uint32) uint32 {
	if id == hovered {
		return 0x424b52
	}
	return normal
}
func (c canvas) button(id string, x, y, w, h int, title string, on bool) {
	col, edge, txt := uint32(raised), uint32(border), uint32(muted)
	if on {
		col, edge, txt = amberTint, 0x73604e, amber
	}
	c.card(x, y, w, h, 6, hi(id, col), edge)
	c.label(x, y, w, h, title, 13, 600, txt, 1, false)
	add(id, x, y, w, h)
}
func (c canvas) toggle(id string, x, y int, on bool) {
	col, knob := uint32(0x424c53), uint32(0xb8c0c3)
	off := 3
	if on {
		col, knob, off = amber, 0x292725, 17
	}
	c.card(x, y, 36, 21, 10, hi(id, col), col)
	c.circle(x+off, y+3, 15, knob, knob)
}
func (c canvas) section(x, y int, num, title string) {
	c.mono(x, y, 27, 22, num, 13, amber)
	c.text(x+29, y, 225, 22, title, 14, 650, ink)
}

func (c canvas) header() {
	c.card(26, 16, 42, 42, 10, amber, amber)
	logo := reticle{Size: 8, Width: 3, Gap: 3, Opacity: 100, Color: 0x262522, Center: true}
	c.reticle(47, 37, 0, logo, 1)
	c.text(81, 8, 200, 37, "Rusthair", 30, 700, ink)
	c.mono(83, 43, 290, 16, "МАСТЕРСКАЯ ПРИЦЕЛОВ", 11, dim)
	c.fill(252, 19, 1, 36, border)
	c.fill(273, 18, 38, 38, border)
	creatorPhoto.paint(c, "creator", 275, 20, 34, 34)
	c.mono(323, 14, 150, 19, "СОЗДАТЕЛЬ", 11, dim)
	nameColor := uint32(ink)
	if hovered == "creator" || keyboardFocus && focused == "creator" {
		nameColor = amber
	}
	c.text(322, 32, 146, 24, "витя гантрап", 16, 600, nameColor)
	add("creator", 269, 14, 180, 46)
	c.card(508, 20, 220, 35, 7, hi("startupHeader", panel), border)
	c.text(520, 20, 159, 35, "Автозапуск с Windows", 14, 600, ink)
	c.toggle("startupHeader", 681, 27, startupEnabled)
	add("startupHeader", 508, 20, 220, 35)
	body, edge, dot := uint32(0x222a2c), uint32(0x394a43), uint32(0x91eab5)
	title := "Прицел включён"
	if overlayFailed {
		title = "Ошибка отрисовки"
		body, edge, dot = amberTint, 0x765847, amber
	}
	if !state.Enabled {
		title = "Прицел выключен"
		body, edge, dot = raised, border, dim
	}
	c.card(742, 20, 192, 35, 17, hi("power", body), edge)
	c.circle(756, 34, 7, dot, dot)
	c.text(774, 20, 145, 35, title, 14, 600, ink)
	add("power", 742, 20, 192, 35)
	c.fill(0, 73, editorW, 1, border)
}

func (c canvas) preview() {
	c.section(26, 88, "01", "Предпросмотр")
	x, y, w, h := 26, 121, 482, 224
	scene := scenes[state.Scene]
	c.label(x+275, 88, w-275, 22, fmt.Sprintf("RUST  /  %02d ИЗ %02d", state.Scene+1, len(scenes)), 11, 500, dim, 2, true)
	if !scenePhoto.paint(c, scene.file, x, y, w, h) {
		c.fill(x, y, w, h, panel)
		c.text(x+23, y+24, w-46, 24, "Не удалось открыть скриншот", 14, 500, muted)
	}
	// Finder edge ticks stay outside the sight picture.
	for i := 0; i < 4; i++ {
		sx, sy, dx, dy := x+12, y+12, 1, 1
		if i&1 != 0 {
			sx = x + w - 13
			dx = -1
		}
		if i&2 != 0 {
			sy = y + h - 13
			dy = -1
		}
		c.line(sx, sy, sx+dx*12, sy, 0xc3bcb3)
		c.line(sx, sy, sx, sy+dy*12, 0xc3bcb3)
	}
	for i := -4; i <= 4; i++ {
		xx := x + w/2 + i*19
		tick := 3
		if i == 0 {
			tick = 7
		}
		c.line(xx, y+12, xx, y+12+tick, 0xa8a9a7)
	}
	zoom := 1.0
	zoomLabel := "×1"
	if state.Zoom {
		zoom = 3
		zoomLabel = "×3"
	}
	// Zoom is relative to physical pixels, even on a scaled desktop.
	saved, _, _ := gdi.NewProc("SaveDC").Call(c.dc)
	clip := c.box(x, y, w, h)
	gdi.NewProc("IntersectClipRect").Call(c.dc, uintptr(clip.L), uintptr(clip.T), uintptr(clip.R), uintptr(clip.B))
	c.reticle(x+w/2, y+h/2, state.Selected, state.Styles[state.Selected], zoom/c.scale)
	gdi.NewProc("RestoreDC").Call(c.dc, saved)
	c.button("zoom", x+23, y+h-42, 44, 26, zoomLabel, false)
	c.button("scenePrev", x+w-254, y+h-42, 30, 26, "‹", false)
	c.button("sceneList", x+w-220, y+h-42, 163, 26, scene.name+"  ▾", false)
	c.button("sceneNext", x+w-53, y+h-42, 30, 26, "›", false)
}
func (c canvas) gallery() {
	c.section(26, 364, "02", "Библиотека")
	c.label(380, 364, 128, 22, "06 ПРЕСЕТОВ", 11, 400, dim, 2, true)
	for i, p := range library {
		x, y := 26+(i%3)*164, 399+(i/3)*84
		id := fmt.Sprintf("preset%d", i)
		on := i == state.Selected
		fill, edge := uint32(panel), uint32(border)
		if on {
			fill, edge = amberTint, amber
		}
		c.card(x, y, 154, 74, 7, hi(id, fill), edge)
		r := state.Styles[i]
		r.Size = 8
		r.Width = 2
		r.Gap = 3
		r.Opacity = 100
		r.Outline = false
		if i == 1 {
			r.Size = 6
		}
		c.reticle(x+24, y+24, i, r, 1)
		c.mono(x+120, y+9, 27, 16, fmt.Sprintf("%02d", i+1), 11, dim)
		c.text(x+14, y+44, 133, 22, p.Name, 15, 600, ink)
		add(id, x, y, 154, 74)
	}
}

type sliderSpec struct {
	id, label, unit string
	low, high       int
	value           *int
	enabled         bool
}

func sliders() []sliderSpec {
	r := &state.Styles[state.Selected]
	if displayTab {
		return []sliderSpec{
			{"offsetX", "По горизонтали", "px", -150, 150, &state.OffsetX, true},
			{"offsetY", "По вертикали", "px", -150, 150, &state.OffsetY, true},
		}
	}
	return []sliderSpec{
		{"size", "Размер", "px", 1, 36, &r.Size, true},
		{"width", "Толщина", "px", 1, 10, &r.Width, state.Selected != 1},
		{"gap", "Зазор", "px", 0, 24, &r.Gap, state.Selected == 0 || state.Selected == 3},
		{"opacity", "Непрозрачность", "%", 5, 100, &r.Opacity, true},
	}
}

const trackX = 553
const trackW = 360

func (c canvas) slider(s sliderSpec, y int) {
	enabled := s.enabled
	col, track, thumb := uint32(muted), uint32(amber), uint32(ink)
	val := fmt.Sprintf("%d %s", *s.value, s.unit)
	if !enabled {
		col, track, thumb = 0x77838b, border, 0x59636b
		val = "—"
	}
	c.text(trackX, y, 205, 22, s.label, 14, 500, col)
	c.label(trackX+230, y, 130, 22, val, 13, 500, col, 2, true)
	px := int(math.Round(float64(*s.value-s.low) / float64(s.high-s.low) * trackW))
	c.fill(trackX, y+30, trackW, 3, border)
	if enabled {
		c.fill(trackX, y+30, px, 3, track)
	}
	c.circle(trackX+px-5, y+26, 11, thumb, thumb)
	if enabled {
		add(s.id, trackX-6, y+19, trackW+12, 23)
	}
}
func (c canvas) editor() {
	c.section(532, 88, "03", "Настройки")
	c.button("styleTab", 747, 85, 88, 28, "Прицел", !displayTab)
	c.button("displayTab", 839, 85, 95, 28, "Экран", displayTab)
	c.card(532, 121, 402, 436, 9, panel, border)
	if displayTab {
		c.displaySettings()
		return
	}
	r := state.Styles[state.Selected]
	p := library[state.Selected]
	c.text(553, 135, 255, 38, p.Name, 29, 650, ink)
	c.text(554, 174, 342, 22, p.Description, 14, 400, muted)
	c.button("resetStyle", 834, 143, 79, 26, "Сбросить", false)
	c.fill(553, 205, 360, 1, border)
	c.text(553, 219, 65, 29, "Цвет", 14, 600, ink)
	for i, col := range swatches {
		x := 615 + i*30
		if r.Color == col {
			c.circle(x-3, 223, 24, ink, ink)
			c.circle(x-1, 225, 20, panel, panel)
		}
		c.circle(x+1, 227, 16, col, col)
		add(fmt.Sprintf("color%d", i), x-4, 220, 26, 30)
	}
	c.button("custom", 804, 220, 109, 29, fmt.Sprintf("#%06X  +", r.Color), false)
	for i, s := range sliders() {
		c.slider(s, 260+49*i)
	}
	c.fill(553, 457, 360, 1, border)
	c.text(553, 470, 230, 30, "Тёмная обводка", 14, 500, ink)
	c.toggle("outline", 877, 475, r.Outline)
	add("outline", 553, 467, 360, 34)
	col := uint32(ink)
	if state.Selected == 1 {
		col = dim
	}
	c.text(553, 509, 260, 30, "Точка в центре", 14, 500, col)
	if state.Selected == 1 {
		c.mono(879, 510, 36, 27, "—", 14, dim)
	} else {
		c.toggle("center", 877, 514, r.Center)
		add("center", 553, 506, 360, 34)
	}
}
func (c canvas) displaySettings() {
	c.text(553, 135, 340, 38, "Положение", 28, 650, ink)
	c.text(554, 175, 360, 23, "Экран и смещение от его центра", 14, 400, muted)
	c.fill(553, 205, 360, 1, border)
	i := selectedDisplay()
	d := displays[i]
	c.text(553, 217, 170, 21, "Монитор", 14, 600, ink)
	id := "monitor"
	if len(displays) == 1 {
		id = ""
	}
	label := fmt.Sprintf("Экран %d   ·   %d × %d", i+1, d.bounds.width(), d.bounds.height())
	if d.primary {
		label += "   ·   основной"
	}
	c.card(553, 246, 360, 39, 6, hi(id, raised), border)
	c.text(566, 246, 321, 39, label, 14, 500, ink)
	if id != "" {
		c.text(890, 246, 20, 39, "›", 19, 500, amber)
		add(id, 553, 246, 360, 39)
	}
	for i, s := range sliders() {
		c.slider(s, 299+i*49)
	}
	c.button("resetPos", 553, 397, 158, 31, "Вернуть в центр", false)
	c.fill(553, 438, 360, 1, border)
	c.text(553, 446, 308, 25, "Запускать вместе с Windows", 14, 600, ink)
	c.toggle("startup", 877, 449, startupEnabled)
	c.text(553, 470, 360, 20, "При входе в систему меню будет свёрнуто.", 13, 400, muted)
	add("startup", 553, 445, 360, 45)
	c.fill(553, 498, 360, 1, border)
	c.text(553, 503, 359, 24, "Окно или безрамочный режим", 14, 600, ink)
	c.text(553, 526, 360, 22, "Сверните редактор, чтобы перейти к игре.", 13, 400, muted)
}
func (c canvas) footer() {
	c.fill(0, 578, 960, 1, border)
	x := 26
	key := func(shortcut, label string, w int) {
		c.card(x, 594, 94, 23, 4, panel, border)
		c.label(x, 594, 94, 23, shortcut, 11, 500, muted, 1, true)
		c.text(x+102, 592, w, 26, label, 13, 400, dim)
		x += 106 + w
	}
	key(hotOverlay, "прицел", 65)
	key(hotMenu, "меню", 55)
	msg := statusText
	if msg == "" {
		msg = "Локальные настройки"
	}
	c.label(556, 591, 378, 28, msg, 12, 400, dim, 2, false)
}

func paintEditor(hwnd uintptr) {
	var ps paintStruct
	dc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer endPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc rect
	getClient.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	if rc.width() < 1 || rc.height() < 1 {
		return
	}
	s := newSurface(rc.width(), rc.height())
	if s == nil {
		return
	}
	defer s.close()
	brushColor.Call(s.dc, colorRef(bg))
	fillRect.Call(s.dc, uintptr(unsafe.Pointer(&rc)), dcBrush)
	displayScale = math.Min(float64(rc.width())/editorW, float64(rc.height())/editorH)
	originX = (rc.width() - int(math.Round(editorW*displayScale))) / 2
	originY = (rc.height() - int(math.Round(editorH*displayScale))) / 2
	userMode := gdi.NewProc("SetViewportOrgEx")
	userMode.Call(s.dc, u(originX), u(originY), 0)
	gdi.NewProc("SetBkMode").Call(s.dc, 1)
	c := canvas{s.dc, displayScale}
	controls = controls[:0]
	c.header()
	c.preview()
	c.gallery()
	c.editor()
	c.footer()
	if keyboardFocus {
		for _, r := range controls {
			if r.id == focused {
				rc := c.box(r.x-2, r.y-2, r.w+4, r.h+4)
				user.NewProc("DrawFocusRect").Call(s.dc, uintptr(unsafe.Pointer(&rc)))
				break
			}
		}
	}
	userMode.Call(s.dc, 0, 0, 0)
	bitBlt.Call(dc, 0, 0, u(s.w), u(s.h), s.dc, 0, 0, 0xcc0020)
}

func stepSlider(id string, delta int) bool {
	for _, s := range sliders() {
		if s.id == id && s.enabled {
			*s.value = clamp(*s.value+delta, s.low, s.high)
			changed()
			return true
		}
	}
	return false
}
func slideTo(id string, x int) bool {
	for _, s := range sliders() {
		if s.id == id && s.enabled {
			v := s.low + int(math.Round(float64(clamp(x-trackX, 0, trackW))*float64(s.high-s.low)/trackW))
			if *s.value != v {
				*s.value = v
				changed()
			}
			return true
		}
	}
	return false
}
func activate(id string) {
	r := &state.Styles[state.Selected]
	switch id {
	case "creator":
		const profile = "https://steamcommunity.com/id/misha_litvin/"
		result, _, _ := openWithShell.Call(mainWindow, uintptr(unsafe.Pointer(str("open"))), uintptr(unsafe.Pointer(str(profile))), 0, 0, 1)
		if result <= 32 {
			statusText = "Не удалось открыть Steam-профиль в браузере"
			repaint()
		}
		return
	case "power":
		state.Enabled = !state.Enabled
	case "zoom":
		state.Zoom = !state.Zoom
	case "sceneNext":
		state.Scene = (state.Scene + 1) % len(scenes)
	case "scenePrev":
		state.Scene = (state.Scene + len(scenes) - 1) % len(scenes)
	case "sceneList":
		i, ok := chooseScene()
		if !ok || i == state.Scene {
			return
		}
		state.Scene = i
	case "styleTab", "displayTab":
		displayTab = id == "displayTab"
		if displayTab {
			refreshStartup()
		}
		repaint()
		return
	case "startup", "startupHeader":
		if err := setStartup(!startupEnabled); err != nil {
			statusText = "Ошибка автозапуска: " + err.Error()
		} else {
			startupEnabled = !startupEnabled
			if startupEnabled {
				statusText = "Автозапуск включён"
			} else {
				statusText = "Автозапуск выключен"
			}
		}
		repaint()
		return
	case "outline":
		r.Outline = !r.Outline
	case "center":
		r.Center = !r.Center
	case "resetStyle":
		*r = library[state.Selected].Default
	case "resetPos":
		state.OffsetX, state.OffsetY = 0, 0
	case "monitor":
		state.Monitor = displays[(selectedDisplay()+1)%len(displays)].name
	case "custom":
		if !pickColor(r) {
			return
		}
	default:
		if strings.HasPrefix(id, "preset") {
			var i int
			if _, err := fmt.Sscanf(id, "preset%d", &i); err != nil || i < 0 || i >= len(library) {
				return
			}
			state.Selected = i
		} else if strings.HasPrefix(id, "color") {
			var i int
			if _, err := fmt.Sscanf(id, "color%d", &i); err != nil || i < 0 || i >= len(swatches) {
				return
			}
			r.Color = swatches[i]
		} else {
			return
		}
	}
	changed()
}
