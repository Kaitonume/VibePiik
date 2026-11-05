//go:build !windows

package appwindow

// Manager is unavailable outside Windows: the App keeps the system browser.
type Manager struct{}

func NewManager(func(string) error, func()) *Manager { return &Manager{} }
func (*Manager) Available() bool                     { return false }
func (*Manager) Open(string) error                   { return ErrUnavailable }
func (*Manager) Close()                              {}

func RestoreWindowPlacement(uintptr, int) {}
func SaveWindowPlacement(uintptr)         {}
func SetDarkTitleBar(uintptr, bool)       {}
