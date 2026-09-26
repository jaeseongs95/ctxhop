//go:build windows

package desktopbundle

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"unsafe"
)

func isReparse(name string, info os.FileInfo) (bool, error) {
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	attributes, err := windows.GetFileAttributes(path)
	return attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, err
}

// Same-directory native rename is atomic, refuses replacement, and works on
// filesystems that do not implement hard links. Handles keep it within Root.
func publishNoReplace(root *os.Root, staging, destination string) error {
	parent, err := root.OpenRoot(filepath.Dir(destination))
	if err != nil {
		return err
	}
	defer parent.Close()
	directory, err := parent.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	name, err := windows.NewNTUnicodeString(filepath.Base(staging))
	if err != nil {
		return err
	}
	oa := windows.OBJECT_ATTRIBUTES{
		RootDirectory: windows.Handle(directory.Fd()), ObjectName: name,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, windows.DELETE|windows.SYNCHRONIZE, &oa, &status, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	utf16, err := windows.UTF16FromString(filepath.Base(destination))
	if err != nil {
		return err
	}
	// This native layout follows x/sys/windows' own rename test. A zero flags
	// value intentionally omits FILE_RENAME_REPLACE_IF_EXISTS.
	type renameInformation struct {
		Flags          uint32
		RootDirectory  windows.Handle
		FileNameLength uint32
		FileName       [1]uint16
	}
	var layout renameInformation
	length := (len(utf16) - 1) * 2
	buffer := make([]byte, int(unsafe.Offsetof(layout.FileName))+length)
	info := (*renameInformation)(unsafe.Pointer(&buffer[0]))
	info.RootDirectory = windows.Handle(directory.Fd())
	info.FileNameLength = uint32(length)
	copy(unsafe.Slice(&info.FileName[0], len(utf16)-1), utf16)
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
}
