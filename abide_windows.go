package abide

import (
	"path/filepath"
	"strings"
)

func isSnapshot(path string) bool {
	return strings.EqualFold(filepath.Ext(path), snapshotExt)
}
