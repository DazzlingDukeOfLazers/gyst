package location

import "syscall"

func fillVolume(path string, env *Env) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return
	}
	env.FSType = cstring(st.Fstypename[:])
	env.Mount = cstring(st.Mntonname[:])
}

func cstring(b []int8) string {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		out = append(out, byte(c))
	}
	return string(out)
}
