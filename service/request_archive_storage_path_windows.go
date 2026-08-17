//go:build windows

package service

import (
	"errors"

	"golang.org/x/sys/windows"
)

func validateRequestArchiveLocalPlatformPath(value string) error {
	pathPointer, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return errors.New("本地请求归档存储路径无效")
	}
	attributes, err := windows.GetFileAttributes(pathPointer)
	if err != nil {
		return err
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("请求归档本地目录不能包含重解析点")
	}
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	length, err := windows.GetLongPathName(pathPointer, &buffer[0], uint32(len(buffer)))
	if err != nil || length == 0 || int(length) >= len(buffer) {
		return errors.New("无法解析请求归档本地目录的完整路径")
	}
	// 8.3 短名称和长名称可能指向同一目录。是否为同一对象由调用方
	// 在受限目录句柄中通过 os.SameFile 校验，不能只按字符串拒绝。

	return nil
}
