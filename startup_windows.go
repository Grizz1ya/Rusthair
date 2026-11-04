package main

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const startupFlag = "--startup"
const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupValue = "Rusthair"

var registry = systemDLL("advapi32.dll")
var startupEnabled bool

func startupCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return `"` + exe + `" ` + startupFlag, nil
}

func readStartup() (bool, error) {
	// Windows limits Run commands to 260 characters, plus the terminating NUL.
	var buf [261]uint16
	size := uint32(unsafe.Sizeof(buf))
	code, _, _ := registry.NewProc("RegGetValueW").Call(syscall.HKEY_CURRENT_USER,
		uintptr(unsafe.Pointer(str(runKey))), uintptr(unsafe.Pointer(str(startupValue))),
		2, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))) // RRF_RT_REG_SZ
	if code == uintptr(syscall.ERROR_FILE_NOT_FOUND) || code == uintptr(syscall.ERROR_PATH_NOT_FOUND) {
		return false, nil
	}
	if code != 0 {
		return false, syscall.Errno(code)
	}
	want, err := startupCommand()
	return strings.EqualFold(syscall.UTF16ToString(buf[:]), want), err
}

func setStartup(enabled bool) error {
	var code uintptr
	if enabled {
		cmd, err := startupCommand()
		if err != nil {
			return err
		}
		data, err := syscall.UTF16FromString(cmd)
		if err != nil {
			return err
		}
		if len(data)-1 > 260 {
			return errors.New("слишком длинный путь к EXE")
		}
		code, _, _ = registry.NewProc("RegSetKeyValueW").Call(syscall.HKEY_CURRENT_USER,
			uintptr(unsafe.Pointer(str(runKey))), uintptr(unsafe.Pointer(str(startupValue))),
			syscall.REG_SZ, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	} else {
		code, _, _ = registry.NewProc("RegDeleteKeyValueW").Call(syscall.HKEY_CURRENT_USER,
			uintptr(unsafe.Pointer(str(runKey))), uintptr(unsafe.Pointer(str(startupValue))))
		if code == uintptr(syscall.ERROR_FILE_NOT_FOUND) || code == uintptr(syscall.ERROR_PATH_NOT_FOUND) {
			return nil
		}
	}
	if code != 0 {
		return syscall.Errno(code)
	}
	return nil
}

func refreshStartup() {
	enabled, err := readStartup()
	if err != nil {
		statusText = "Не удалось прочитать настройку автозапуска"
	} else {
		startupEnabled = enabled
	}
}
