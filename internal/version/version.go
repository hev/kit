// Package version is the release this binary was built from. The release
// build sets it with
//
//	-ldflags "-X github.com/hev/kit/internal/version.Version=0.1.0"
//
// and everything else is "dev".
package version

// Version is the release number without a leading "v", or "dev".
var Version = "dev"

// KitImage is the dashboard image a binary pulls by default: the one built
// from the same tag, so `hev up` never pairs a daemon with a dashboard from
// another release. A dev build takes the newest release.
func KitImage() string {
	if Version == "" || Version == "dev" {
		return "hevlayer/kit:latest"
	}
	return "hevlayer/kit:" + Version
}
