package localfolder

import (
	"io/fs"
	"syscall"
)

// SF_DATALESS: the file's content is held by a File Provider (iCloud Drive,
// OneDrive, Dropbox, Google Drive) and is not on disk. Opening it triggers a
// download, which a read-only scanner must never cause.
const sfDataless = 0x40000000

func isPlaceholder(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Flags&sfDataless != 0
}
