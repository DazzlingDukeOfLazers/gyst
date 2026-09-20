package location

import (
	"bufio"
	"os"
	"strings"
	"syscall"
)

// Filesystem magic numbers from statfs(2), for when /proc is unavailable.
var linuxMagic = map[int64]string{
	0xFF534D42: "cifs", 0xFE534D42: "smb3", 0x6969: "nfs", 0x65735546: "fuse",
	0xEF53: "ext4", 0x58465342: "xfs", 0x9123683E: "btrfs", 0x01021994: "tmpfs",
	0x5346544E: "ntfs", 0x4D44: "vfat", 0x2FC12FC1: "zfs", 0x0187: "autofs",
	0x794C7630: "overlay", 0x3153464A: "jfs", 0x52654973: "reiserfs",
}

func fillVolume(path string, env *Env) {
	// /proc/self/mounts gives the exact type ("fuse.sshfs") and the mount
	// point; statfs alone gives only a magic number.
	if f, err := os.Open("/proc/self/mounts"); err == nil {
		defer f.Close()
		best := ""
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 3 {
				continue
			}
			mp := unescapeMount(fields[1])
			if under(path, mp) && len(mp) > len(best) {
				best, env.Mount, env.FSType = mp, mp, fields[2]
			}
		}
		if best != "" {
			return
		}
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return
	}
	env.FSType = linuxMagic[int64(st.Type)]
}

// unescapeMount decodes the octal escapes /proc/mounts uses for spaces.
func unescapeMount(s string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(s)
}
