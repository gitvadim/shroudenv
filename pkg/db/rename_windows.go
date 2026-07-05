//go:build windows

package db

import (
	"syscall"
	"unsafe"
)

var (
	modkernel32     = syscall.NewLazyDLL("kernel32.dll")
	procMoveFileExW = modkernel32.NewProc("MoveFileExW")
)

func renameWrap(src, dst string) error {
	srcPtr, err := syscall.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	dstPtr, err := syscall.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}

	// MOVEFILE_REPLACE_EXISTING = 0x1
	// MOVEFILE_WRITE_THROUGH = 0x8
	const flags = 0x1 | 0x8

	r1, _, err := procMoveFileExW.Call(
		uintptr(unsafe.Pointer(srcPtr)),
		uintptr(unsafe.Pointer(dstPtr)),
		uintptr(flags),
	)
	if r1 == 0 {
		// Win32 APIs return 0 on failure; err is populated with the last error code
		if err != nil {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
