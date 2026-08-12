//go:build linux || darwin

package tui

import (
	"syscall"
	"unsafe"
)

// makeRaw puts the tty into raw-enough mode: no line buffering, no echo, no
// signal generation (Ctrl-C is handled as a key so the deferred restores
// always run). Returns the function that restores the previous state.
func makeRaw(fd uintptr) (func(), error) {
	var old syscall.Termios
	if err := ioctl(fd, ioctlGetTermios, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, ioctlSetTermios, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return func() { _ = ioctl(fd, ioctlSetTermios, unsafe.Pointer(&old)) }, nil
}

func ioctl(fd, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}
