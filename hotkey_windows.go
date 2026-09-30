//go:build windows

package main

import "golang.org/x/sys/windows"

var (
	user32DLL            = windows.NewLazySystemDLL("user32.dll")
	procGetAsyncKeyState = user32DLL.NewProc("GetAsyncKeyState")
)

const vkEnd = 0x23

func endKeyPressed() bool {
	state, _, _ := procGetAsyncKeyState.Call(vkEnd)
	return state&0x8000 != 0
}
