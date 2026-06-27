// Package buildinfo exposes build metadata injected at link time via
// -ldflags "-X github.com/UzStack/jst-go/internal/shared/buildinfo.Version=...".
package buildinfo

// Version is the build version; defaults to "dev" for local builds.
var Version = "dev"
