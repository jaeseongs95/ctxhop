//go:build !windows

package desktopbundle

import "os"

func isReparse(name string, info os.FileInfo) (bool, error) {
	return info.Mode()&os.ModeSymlink != 0, nil
}

func publishNoReplace(root *os.Root, staging, destination string) error {
	return root.Link(staging, destination)
}
