package version

import (
	"fmt"
	"runtime"
)

// Version values can be injected at build time via -ldflags:
//
//	-X github.com/Harish-vinayagam/Skall/internal/version.Version=1.0.0
//	-X github.com/Harish-vinayagam/Skall/internal/version.GitCommit=abcdef
//	-X github.com/Harish-vinayagam/Skall/internal/version.BuildDate=2026-09-17
var (
	// Version is the current semver release version.
	Version = "1.0.0"

	// GitCommit is the git SHA from which the binary was built.
	GitCommit = "dev"

	// BuildDate is the UTC ISO date string when the binary was built.
	BuildDate = "unknown"
)

// BuildInfo holds complete version and platform metadata.
type BuildInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// Info returns the current BuildInfo snapshot.
func Info() BuildInfo {
	return BuildInfo{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

// String returns a human-readable summary of the version and build environment.
func String() string {
	info := Info()
	return fmt.Sprintf("SKALL v%s (commit: %s, built: %s, %s %s/%s)",
		info.Version, info.GitCommit, info.BuildDate, info.GoVersion, info.OS, info.Arch)
}

// Short returns only the version string prefixed with 'v'.
func Short() string {
	return "v" + Version
}
