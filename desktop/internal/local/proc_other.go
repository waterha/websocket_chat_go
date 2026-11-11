//go:build !windows

package local

import "os/exec"

func hideWindow(*exec.Cmd)                     {}
func postgresPath(path string) (string, error) { return path, nil }
