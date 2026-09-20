//go:build !darwin && !windows

package localfolder

import "io/fs"

func isPlaceholder(info fs.FileInfo) bool { return false }
