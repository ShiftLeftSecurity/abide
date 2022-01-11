//go:build !windows
// +build !windows

package abide

import "path/filepath"

func isSnapshot(path string) bool {
	return filepath.Ext(path) == snapshotExt
}
