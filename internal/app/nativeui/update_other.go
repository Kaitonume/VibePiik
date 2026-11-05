//go:build !windows

package nativeui

import (
	"os/exec"
	"time"
)

func detachConsole(*exec.Cmd)        {}
func waitForExit(int, time.Duration) { time.Sleep(2 * time.Second) }
