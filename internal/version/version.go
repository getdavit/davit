// Package version holds the build-time version string injected via ldflags.
package version

// Version is set at build time via:
//
//	go build -ldflags="-X github.com/getdavit/davit/internal/version.Version=v0.2.0"
var Version = "dev"
