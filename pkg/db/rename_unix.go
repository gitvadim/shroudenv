//go:build !windows

package db

import "os"

func renameWrap(src, dst string) error {
	return os.Rename(src, dst)
}
