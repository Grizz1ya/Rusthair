package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type reticle struct {
	Size, Width, Gap, Opacity int
	Color                     uint32 // 0xRRGGBB
	Outline, Center           bool
}

type preset struct {
	Name, Description string
	Default           reticle
}

var library = []preset{
	{"Классика", "Четыре штриха с открытым центром", reticle{9, 2, 3, 100, 0x8ffcc0, true, false}},
	{"Точка", "Одна метка для точного наведения", reticle{5, 2, 0, 100, 0xffbc76, true, false}},
	{"Кольцо", "Тонкая окружность вокруг цели", reticle{9, 2, 0, 100, 0x7ddcff, true, true}},
	{"Т-форма", "Открытый обзор над целью", reticle{9, 2, 3, 100, 0xf8f4e9, true, false}},
	{"Шеврон", "Вершина угла указывает на цель", reticle{9, 2, 0, 100, 0xff8da5, true, false}},
	{"Ромб", "Четыре грани и свободный центр", reticle{10, 2, 0, 100, 0xdfb1ff, true, false}},
}

var swatches = []uint32{0x8ffcc0, 0x7ddcff, 0xffbc76, 0xff8da5, 0xdfb1ff, 0xf8f4e9}

// Keep dusk, night and day at their original settings indices.
var scenes = []struct{ file, name string }{
	{"ocean", "Океан на закате"},
	{"night", "Ночной лес"},
	{"valley", "Долина днём"},
	{"river", "Лесная река"},
	{"oasis", "Пустынный оазис"},
	{"arctic", "Арктическая база"},
	{"jungle", "Джунгли"},
	{"underwater", "Под водой"},
	{"radtown", "Радтаун"},
	{"ziggurat", "Зиккурат"},
}

type settings struct {
	Version          int
	Selected         int
	Styles           []reticle
	Enabled          bool
	Monitor          string
	OffsetX, OffsetY int
	Scene            int
	Zoom             bool
}

func defaults() settings {
	s := settings{Version: 1, Enabled: true, Zoom: true}
	for _, p := range library {
		s.Styles = append(s.Styles, p.Default)
	}
	return s
}

func clamp(v, low, high int) int { return min(max(v, low), high) }

func settingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "Rusthair", "settings.json"), nil
}

func loadSettings() (settings, error) {
	s := defaults()
	path, err := settingsPath()
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	var loaded settings
	if err = json.Unmarshal(data, &loaded); err != nil {
		return s, err
	}
	if loaded.Version != 1 {
		return s, errors.New("неизвестная версия настроек")
	}
	loaded.Selected = clamp(loaded.Selected, 0, len(library)-1)
	loaded.OffsetX = clamp(loaded.OffsetX, -150, 150)
	loaded.OffsetY = clamp(loaded.OffsetY, -150, 150)
	loaded.Scene = clamp(loaded.Scene, 0, len(scenes)-1)
	for i, r := range loaded.Styles {
		if i >= len(s.Styles) {
			break
		}
		r.Size = clamp(r.Size, 1, 36)
		r.Width = clamp(r.Width, 1, 10)
		r.Gap = clamp(r.Gap, 0, 24)
		r.Opacity = clamp(r.Opacity, 5, 100)
		r.Color &= 0xffffff
		s.Styles[i] = r
	}
	loaded.Styles = s.Styles
	return loaded, nil
}

func saveSettings(s settings) error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
