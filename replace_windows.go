package cacheddownloader

import (
       "syscall"
       "unsafe"
)

func Replace(src, dst string) error {
	kernel32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		return err
	}
	defer syscall.FreeLibrary(kernel32)
	moveFileExUnicode, err := syscall.GetProcAddress(kernel32, "MoveFileExW")
	if err != nil {
		return err
	}

	srcString := syscall.StringToUTF16Ptr(src)
	dstString := syscall.StringToUTF16Ptr(dst)

	srcPtr := uintptr(unsafe.Pointer(srcString))
	dstPtr := uintptr(unsafe.Pointer(dstString))

	MOVEFILE_REPLACE_EXISTING := 0x1
	flag := uintptr(MOVEFILE_REPLACE_EXISTING)

	_, _, callErr := syscall.Syscall(uintptr(moveFileExUnicode), 3, srcPtr, dstPtr, flag)
	if callErr != 0 {
		return callErr
	}

	return nil
}