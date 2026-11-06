// Package buildinfo holds values stamped into the binary at build time.
package buildinfo

// Version is the release version, set with
// -ldflags "-X github.com/shurastik/agterm-kit/internal/buildinfo.Version=X.Y.Z".
var Version = "dev"
