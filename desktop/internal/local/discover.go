package local

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// findExecutable mirrors the search strategy of scripts/dev-env.ps1: explicit
// environment overrides first, then PATH, then the standard install bases and
// the top level of every drive. The first directory that contains the
// executable (directly, in bin/ or in a matching subdirectory) wins.
func findExecutable(executable string, explicit []string, patterns []string) string {
	candidates := append([]string{}, explicit...)
	if path, err := exec.LookPath(executable); err == nil {
		candidates = append(candidates, filepath.Dir(path))
	}
	for _, base := range installBases() {
		candidates = append(candidates, topLevelMatches(base, patterns)...)
	}
	for _, root := range driveRoots() {
		candidates = append(candidates, topLevelMatches(root, patterns)...)
	}
	for _, candidate := range candidates {
		if directory := directoryContaining(candidate, executable, patterns); directory != "" {
			return directory
		}
	}
	return ""
}

// scanFor runs findExecutable under a watchdog: a stalled drive or network
// path must not block the launcher indefinitely.
func scanFor(executable string, patterns []string) string {
	result := make(chan string, 1)
	go func() { result <- findExecutable(executable, nil, patterns) }()
	select {
	case directory := <-result:
		return directory
	case <-time.After(20 * time.Second):
		return ""
	}
}

// directoryContaining checks one candidate directory for the executable,
// looking at the directory itself, its bin subdirectory and any direct
// subdirectory whose name matches the patterns. Matching is case-insensitive
// because Windows file names are.
func directoryContaining(candidate, executable string, patterns []string) string {
	if candidate == "" {
		return ""
	}
	directories := []string{candidate, filepath.Join(candidate, "bin")}
	for _, match := range topLevelMatches(candidate, patterns) {
		directories = append(directories, match, filepath.Join(match, "bin"))
	}
	for _, directory := range directories {
		if fileExists(filepath.Join(directory, executable)) {
			return directory
		}
	}
	return ""
}

// topLevelMatches lists the direct subdirectories of base whose names match
// any pattern, like the case-insensitive Get-ChildItem -Filter.
func topLevelMatches(base string, patterns []string) []string {
	if base == "" {
		return nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var matches []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		for _, pattern := range patterns {
			if matchName(pattern, entry.Name()) {
				matches = append(matches, filepath.Join(base, entry.Name()))
				break
			}
		}
	}
	return matches
}

func matchName(pattern, name string) bool {
	ok, err := filepath.Match(strings.ToLower(pattern), strings.ToLower(name))
	return ok && err == nil
}

func installBases() []string {
	var bases []string
	for _, variable := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "LOCALAPPDATA"} {
		if base := os.Getenv(variable); base != "" {
			bases = append(bases, base)
		}
	}
	return bases
}

func driveRoots() []string {
	var roots []string
	for letter := 'C'; letter <= 'Z'; letter++ {
		root := string(letter) + `:\`
		if _, err := os.Stat(root); err == nil {
			roots = append(roots, root)
		}
	}
	return roots
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// FindPostgres locates a directory containing pg_ctl. IM_POSTGRES_BIN wins and
// is authoritative; otherwise PATH and the usual PostgreSQL install locations
// are scanned.
func FindPostgres() string {
	if override := os.Getenv("IM_POSTGRES_BIN"); override != "" {
		return directoryContaining(override, exeName("pg_ctl"), nil)
	}
	return scanFor(exeName("pg_ctl"), []string{"PostgreSQL*", "*pgsql*"})
}

// FindRedis locates a directory containing redis-server. IM_REDIS_DIR wins and
// is authoritative; otherwise the launcher's portable copy is checked before
// scanning PATH and the usual locations.
func FindRedis(root string) string {
	executable := exeName("redis-server")
	if override := os.Getenv("IM_REDIS_DIR"); override != "" {
		return directoryContaining(override, executable, []string{"*redis*"})
	}
	if directory := directoryContaining(filepath.Join(root, "redis"), executable, []string{"*redis*"}); directory != "" {
		return directory
	}
	return scanFor(executable, []string{"*redis*"})
}
