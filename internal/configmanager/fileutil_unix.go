//go:build !windows

package configmanager

import (
	"os"
	"syscall"
)

func init() {
	fileLocker.acquire = func(file *os.File) error {
		return syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
	}
	fileLocker.release = func(file *os.File) error {
		return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	}
}
