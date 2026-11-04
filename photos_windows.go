package main

import (
	"embed"
	"image"
	"image/draw"
	"image/jpeg"
	"unsafe"
)

//go:embed media/*.jpg
var photos embed.FS

type photoCache struct {
	bitmap *surface
	file   string
}

// The embedded scenes are cropped to the preview frame at build time. Decode
// and scale only when the user changes the scene or the window's DPI changes.
func (p *photoCache) paint(c canvas, file string, x, y, w, h int) bool {
	width, height := c.p(w), c.p(h)
	if p.bitmap == nil || p.file != file || p.bitmap.w != width || p.bitmap.h != height {
		p.bitmap.close()
		p.bitmap = loadPhoto(file, width, height)
		p.file = file
	}
	if p.bitmap == nil {
		return false
	}
	bitBlt.Call(c.dc, u(c.p(x)), u(c.p(y)), u(width), u(height), p.bitmap.dc, 0, 0, 0xcc0020)
	return true
}

func loadPhoto(file string, width, height int) *surface {
	f, err := photos.Open("media/" + file + ".jpg")
	if err != nil {
		return nil
	}
	defer f.Close()
	im, err := jpeg.Decode(f)
	if err != nil {
		return nil
	}
	b := im.Bounds()
	rgba := image.NewRGBA(b)
	draw.Draw(rgba, b, im, b.Min, draw.Src)
	for i := 0; i < len(rgba.Pix); i += 4 {
		rgba.Pix[i], rgba.Pix[i+2] = rgba.Pix[i+2], rgba.Pix[i]
	}
	dst := newSurface(width, height)
	if dst == nil {
		return nil
	}
	info := bitmapHeader{Size: 40, Width: int32(b.Dx()), Height: -int32(b.Dy()), Planes: 1, BitCount: 32}
	gdi.NewProc("SetStretchBltMode").Call(dst.dc, 4) // HALFTONE
	gdi.NewProc("SetBrushOrgEx").Call(dst.dc, 0, 0, 0)
	lines, _, _ := gdi.NewProc("StretchDIBits").Call(dst.dc, 0, 0, u(width), u(height), 0, 0, u(b.Dx()), u(b.Dy()), uintptr(unsafe.Pointer(&rgba.Pix[0])), uintptr(unsafe.Pointer(&info)), 0, 0xcc0020)
	if int32(lines) <= 0 {
		dst.close()
		return nil
	}
	return dst
}

func chooseScene() (int, bool) {
	menu, _, _ := user.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return 0, false
	}
	defer user.NewProc("DestroyMenu").Call(menu)
	appendItem := user.NewProc("AppendMenuW")
	for i, s := range scenes {
		var flags uintptr
		if i == state.Scene {
			flags = 8
		} // MF_CHECKED
		appendItem.Call(menu, flags, u(i+1), uintptr(unsafe.Pointer(str(s.name))))
	}
	// Anchor the picker to its button for both pointer and keyboard input.
	var anchor point
	for _, r := range controls {
		if r.id == "sceneList" {
			c := canvas{scale: displayScale}
			anchor = point{int32(originX + c.p(r.x)), int32(originY + c.p(r.y+r.h))}
			break
		}
	}
	user.NewProc("ClientToScreen").Call(mainWindow, uintptr(unsafe.Pointer(&anchor)))
	chosen, _, _ := user.NewProc("TrackPopupMenu").Call(menu, 0x0180, uintptr(anchor.X), uintptr(anchor.Y), 0, mainWindow, 0)
	if chosen == 0 || chosen > uintptr(len(scenes)) {
		return 0, false
	}
	return int(chosen) - 1, true
}
