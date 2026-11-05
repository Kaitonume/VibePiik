//go:build windows

package nativeui

import (
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// detachConsole gives the restarted App its own console (the old one closes
// with the old process). Automated tests start it without a window.
func detachConsole(command *exec.Cmd) {
	flags := uint32(windows.CREATE_NEW_CONSOLE)
	if os.Getenv("VIBEPIIK_TEST_UPDATE") != "" {
		flags = windows.CREATE_NO_WINDOW
	}
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
}

func waitForExit(pid int, limit time.Duration) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return // already gone
	}
	defer windows.CloseHandle(handle)
	_, _ = windows.WaitForSingleObject(handle, uint32(limit/time.Millisecond))
}
