package utils

import (
	"math/rand"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"golang.org/x/mod/semver"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func GetRandomString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func MergeSchemas(schemas ...map[string]*schema.Schema) map[string]*schema.Schema {
	result := make(map[string]*schema.Schema)
	for _, m := range schemas {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

func SelectVersion(availableVersions []string, wantedVersion string) string {
	if semver.Major("v"+wantedVersion) == "v"+wantedVersion {
		for _, availableVersion := range availableVersions {
			availableMajor := semver.Major("v" + availableVersion)
			wantedMajor := semver.Major("v" + wantedVersion)
			if semver.Compare(availableMajor, wantedMajor) == 0 {
				return availableVersion
			}
		}
		return ""
	}

	if semver.MajorMinor("v"+wantedVersion) == "v"+wantedVersion {
		for _, availableVersion := range availableVersions {
			wantedMajorMinor := semver.MajorMinor("v" + wantedVersion)
			availableMajorMinor := semver.MajorMinor("v" + availableVersion)

			if strings.Compare(wantedMajorMinor, availableMajorMinor) == 0 {
				return availableVersion
			}
		}

		return ""
	}

	for _, availableVersion := range availableVersions {
		if wantedVersion == availableVersion {
			return availableVersion
		}
		availableNoRevision := removeDebianRevision(availableVersion)
		if semver.Compare("v"+wantedVersion, "v"+availableNoRevision) == 0 {
			return availableVersion
		}
	}
	return ""
}

func removeDebianRevision(version string) string {
	return strings.Split(version, "-")[0]
}

func Ref[T any](x T) *T {
	return &x
}

func ParsePMMAddress(link string) (string, error) {
	addr, err := url.Parse(link)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse pmm address")
	}
	if addr.Scheme == "" {
		return "", errors.New("url scheme is empty")
	}
	switch addr.Scheme {
	case "http":
		if addr.Port() == "" {
			addr.Host += ":80"
		}
	case "https":
		if addr.Port() == "" {
			addr.Host += ":443"
		}
	}
	if addr.User == nil || addr.User.String() == "" {
		addr.User = url.UserPassword("admin", "admin")
	}
	return addr.String(), nil
}
