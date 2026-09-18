package version

import (
	"strings"
	"testing"
)

func TestVersionInfo(t *testing.T) {
	info := Info()
	if info.Version != Version {
		t.Fatalf("Info().Version = %q, want %q", info.Version, Version)
	}
	if info.OS == "" {
		t.Fatal("Info().OS is empty")
	}
	if info.Arch == "" {
		t.Fatal("Info().Arch is empty")
	}
	if info.GoVersion == "" {
		t.Fatal("Info().GoVersion is empty")
	}
}

func TestVersionString(t *testing.T) {
	str := String()
	if !strings.HasPrefix(str, "SKALL v"+Version) {
		t.Fatalf("String() = %q, want prefix %q", str, "SKALL v"+Version)
	}
	if !strings.Contains(str, "commit:") {
		t.Fatalf("String() = %q missing commit info", str)
	}
}

func TestVersionShort(t *testing.T) {
	short := Short()
	if short != "v1.0.0" {
		t.Fatalf("Short() = %q, want v1.0.0", short)
	}
}
