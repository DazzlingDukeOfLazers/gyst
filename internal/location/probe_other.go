//go:build !darwin && !linux && !windows

package location

func fillVolume(path string, env *Env) {}
