package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW                = user32.NewProc("FindWindowW")
	procGetWindowLongPtrW          = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW          = user32.NewProc("SetWindowLongPtrW")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
)

const (
	gwlExStyle     = ^uintptr(19) // GWL_EXSTYLE (-20)
	wsExLayered    = 0x00080000
	wsExToolWindow = 0x00000080 // keeps the widget off the taskbar
	wsExAppWindow  = 0x00040000
	lwaAlpha       = 0x2
)

// setWindowOpacity makes the whole widget window see-through. Wails' own
// transparent webview mode does not tint the desktop behind it, so text behind
// stayed fully visible; a layered window with constant alpha does.
func setWindowOpacity(title string, opacity float64) {
	name, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(name)))
	if hwnd == 0 {
		return
	}
	style, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExStyle)
	style = (style | wsExLayered | wsExToolWindow) &^ wsExAppWindow
	procSetWindowLongPtrW.Call(hwnd, gwlExStyle, style)
	procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(clampOpacity(opacity)*255), lwaAlpha)
}

func clampOpacity(v float64) float64 {
	switch {
	case v <= 0:
		return 0.85 // unset
	case v < 0.3:
		return 0.3
	case v > 1:
		return 1
	}
	return v
}
