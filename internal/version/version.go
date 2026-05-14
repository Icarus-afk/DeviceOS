package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

var Version = "0.1.0-dev"

type BuildInfo struct {
	Version    string
	GoVersion  string
	Commit     string
	CommitDate string
	OS         string
	Arch       string
}

func ReadBuildInfo() BuildInfo {
	info := BuildInfo{
		Version:   Version,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				info.Commit = s.Value
			case "vcs.time":
				info.CommitDate = s.Value
			}
		}
	}
	return info
}

func (b BuildInfo) String() string {
	parts := []string{fmt.Sprintf("deviceos %s", b.Version)}
	if b.Commit != "" {
		commit := b.Commit
		if len(commit) > 8 {
			commit = commit[:8]
		}
		parts = append(parts, fmt.Sprintf("commit %s", commit))
	}
	parts = append(parts, b.GoVersion, fmt.Sprintf("%s/%s", b.OS, b.Arch))
	return strings.Join(parts, " | ")
}

func Banner() string {
	return fmt.Sprintf(`╔══════════════════════════════════════╗
║         DeviceOS v%-18s ║
║   Self-hosted IoT Backend           ║
╚══════════════════════════════════════╝`, Version)
}
