// Package update moves a host's hev to another published build: the newest
// release, the newest edge build of the open train, or a named version. It
// fetches only from the official repository's published builds, verifies the
// asset digest against the build's manifest before anything is replaced,
// installs beside the running binary and switches atomically, keeping the
// build it replaced for rollback.
//
// The design and the trust model are in site/src/content/docs/updating.mdx.
package update

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	ChannelRelease = "release"
	ChannelEdge    = "edge"
)

var (
	releaseVersion = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)
	edgeVersion    = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)-edge\.([0-9a-f]{7,40})$`)
)

// ValidChannel reports whether c names a channel.
func ValidChannel(c string) bool { return c == ChannelRelease || c == ChannelEdge }

// NormalizeVersion accepts X.Y.Z, vX.Y.Z or X.Y.Z-edge.<sha> and returns it
// without the leading v. Anything else, including paths and URLs, is refused:
// a version is the only thing a caller may name.
func NormalizeVersion(v string) (string, error) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if releaseVersion.MatchString(v) || edgeVersion.MatchString(v) {
		return v, nil
	}
	return "", fmt.Errorf("invalid version %q: want X.Y.Z or X.Y.Z-edge.<sha>", v)
}

// VersionChannel is the channel a valid version is published on.
func VersionChannel(v string) string {
	if edgeVersion.MatchString(v) {
		return ChannelEdge
	}
	return ChannelRelease
}

// Tag is the git tag a version is published under. Edge tags do not start
// with "v", so the release scripts' vX.Y.Z tag globs never see them.
func Tag(v string) string {
	if edgeVersion.MatchString(v) {
		return "edge-" + v
	}
	return "v" + v
}

// base returns X.Y.Z as numbers and whether v is a known version.
func base(v string) ([3]int, bool) {
	m := releaseVersion.FindStringSubmatch(v)
	if m == nil {
		m = edgeVersion.FindStringSubmatch(v)
	}
	if m == nil {
		return [3]int{}, false
	}
	var out [3]int
	for i := range out {
		out[i], _ = strconv.Atoi(m[i+1])
	}
	return out, true
}

// Compare orders two versions. ok is false when the order cannot be told
// from the strings alone: either is not a published version (a dev build),
// or both are edge builds of the same X.Y.Z, which only their manifests'
// sequence numbers order. A release sorts after every edge build of its own
// X.Y.Z, since those were built on the way to it.
func Compare(a, b string) (cmp int, ok bool) {
	if a == b {
		return 0, true
	}
	x, okA := base(a)
	y, okB := base(b)
	if !okA || !okB {
		return 0, false
	}
	for i := range x {
		if x[i] != y[i] {
			if x[i] < y[i] {
				return -1, true
			}
			return 1, true
		}
	}
	ea, eb := edgeVersion.MatchString(a), edgeVersion.MatchString(b)
	switch {
	case ea && !eb:
		return -1, true
	case !ea && eb:
		return 1, true
	}
	return 0, false
}

// AssetName is the archive a build publishes for one platform.
func AssetName(version, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("kit_%s_%s_%s%s", version, goos, goarch, ext)
}

// BinaryName is the executable inside an archive.
func BinaryName(goos string) string {
	if goos == "windows" {
		return "hev.exe"
	}
	return "hev"
}
