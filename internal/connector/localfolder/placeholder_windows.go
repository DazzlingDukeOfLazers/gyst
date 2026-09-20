package localfolder

import (
	"io/fs"
	"syscall"
)

// Attributes a sync engine sets on a file whose content is not present
// locally: OneDrive Files On-Demand and other cloud filter providers. Reading
// the file would hydrate it, which is a download the scanner did not ask for.
const (
	fileAttributeOffline            = 0x00001000
	fileAttributeRecallOnOpen       = 0x00040000
	fileAttributeRecallOnDataAccess = 0x00400000
	placeholderAttributes           = fileAttributeOffline | fileAttributeRecallOnOpen | fileAttributeRecallOnDataAccess
)

func isPlaceholder(info fs.FileInfo) bool {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && d.FileAttributes&placeholderAttributes != 0
}
