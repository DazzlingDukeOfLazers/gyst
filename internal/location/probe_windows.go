package location

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	driveUnknown   = 0
	driveNoRoot    = 1
	driveRemovable = 2
	driveFixed     = 3
	driveRemote    = 4
	driveCDROM     = 5
	driveRAMDisk   = 6
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	getDriveTypeW         = kernel32.NewProc("GetDriveTypeW")
	getVolumeInformationW = kernel32.NewProc("GetVolumeInformationW")
)

func fillVolume(path string, env *Env) {
	vol := filepath.VolumeName(path)
	if strings.HasPrefix(vol, `\\`) {
		env.UNC = true
		env.Mount = vol
		return
	}
	if vol == "" {
		return
	}
	root := vol + `\`
	env.Mount = root
	rootp, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return
	}
	dt, _, _ := getDriveTypeW.Call(uintptr(unsafe.Pointer(rootp)))
	switch dt {
	case driveRemote:
		// A mapped drive letter. The share behind it is reported by the
		// filesystem name below only as "NTFS" or similar, so the drive
		// type is the evidence.
		env.FSType = "smb"
		return
	case driveRemovable:
		env.FSType = "removable"
	}
	var fsName [syscall.MAX_PATH + 1]uint16
	var serial, maxLen, flags uint32
	ok, _, _ := getVolumeInformationW.Call(
		uintptr(unsafe.Pointer(rootp)), 0, 0,
		uintptr(unsafe.Pointer(&serial)), uintptr(unsafe.Pointer(&maxLen)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&fsName[0])), uintptr(len(fsName)))
	if ok != 0 {
		if name := strings.ToLower(syscall.UTF16ToString(fsName[:])); name != "" {
			if env.FSType == "removable" {
				env.FSType = name + " (removable)"
			} else {
				env.FSType = name
			}
		}
	}
}
