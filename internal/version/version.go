package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

var (
	Version      = "0.1.0"
	ReleaseName  = "Hummingbird"
	Commit       = ""
)

type BuildInfo struct {
	Version    string
	Commit     string
	CommitDate string
	GoVersion  string
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
	if info.Commit == "" {
		info.Commit = Commit
	}
	return info
}

func (b BuildInfo) Display() string {
	s := fmt.Sprintf("DeviceOS v%s %s", b.Version, ReleaseName)
	c := b.Commit
	if len(c) > 8 {
		c = c[:8]
	}
	if c != "" {
		s += fmt.Sprintf(" (%s)", c)
	}
	return s
}

func (b BuildInfo) String() string {
	return fmt.Sprintf("%s | %s | %s/%s", b.Display(), b.GoVersion, b.OS, b.Arch)
}

func Banner() string {
	line := fmt.Sprintf("DeviceOS v%s %s", Version, ReleaseName)
	if len(line) > 34 {
		line = line[:34]
	}
	padding := 34 - len(line)
	left := padding / 2
	right := padding - left
	return fmt.Sprintf("╔══════════════════════════════════════╗\n║%s%s%s║\n║   Self-hosted IoT Backend           ║\n╚══════════════════════════════════════╝",
		strings.Repeat(" ", left), line, strings.Repeat(" ", right))
}
