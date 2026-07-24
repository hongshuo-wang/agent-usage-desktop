//go:build windows

package config

import (
	"runtime"
	"syscall"
	"unsafe"
)

const (
	moveFileReplaceExisting = 0x1
	moveFileWriteThrough    = 0x8
)

var moveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

var moveConfigFile = func(tempPath, targetPath string) error {
	tempUTF16, err := syscall.UTF16PtrFromString(tempPath)
	if err != nil {
		return err
	}
	targetUTF16, err := syscall.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}
	result, _, callErr := moveFileExW.Call(
		uintptr(unsafe.Pointer(tempUTF16)),
		uintptr(unsafe.Pointer(targetUTF16)),
		moveFileReplaceExisting|moveFileWriteThrough,
	)
	runtime.KeepAlive(tempUTF16)
	runtime.KeepAlive(targetUTF16)
	if result == 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return syscall.EINVAL
	}
	return nil
}

func replaceConfigFile(tempPath, targetPath string) error {
	return moveConfigFile(tempPath, targetPath)
}
