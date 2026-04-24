//go:build windows

package configmanager

import (
	"math"
	"os"

	"golang.org/x/sys/windows"
)

func init() {
	fileLocker.acquire = func(file *os.File) error {
		handle := windows.Handle(file.Fd())
		overlapped := new(windows.Overlapped)
		return windows.LockFileEx(
			handle,
			windows.LOCKFILE_EXCLUSIVE_LOCK,
			0,
			math.MaxUint32,
			math.MaxUint32,
			overlapped,
		)
	}

	fileLocker.release = func(file *os.File) error {
		handle := windows.Handle(file.Fd())
		overlapped := new(windows.Overlapped)
		return windows.UnlockFileEx(
			handle,
			0,
			math.MaxUint32,
			math.MaxUint32,
			overlapped,
		)
	}

	atomicReplace = func(src, dst string) error {
		srcPtr, err := windows.UTF16PtrFromString(src)
		if err != nil {
			return err
		}

		dstPtr, err := windows.UTF16PtrFromString(dst)
		if err != nil {
			return err
		}

		return windows.MoveFileEx(
			srcPtr,
			dstPtr,
			windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
		)
	}
}
