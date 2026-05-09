package vfsrw

import (
	"regexp"
	"strings"

	"emperror.dev/errors"
)

var matchPathRegexp = regexp.MustCompile(`^vfs://?([^/]+)(/(.*))?$`)

func MatchPath(vfsPath string) (name string, path string, err error) {
	if strings.HasPrefix(vfsPath, "vfs:/") {
		matches := matchPathRegexp.FindStringSubmatch(vfsPath)
		if matches == nil {
			return "", "", errors.Errorf("invalid vfs path: %s", vfsPath)
		}
		name = matches[1]
		path = matches[3]
		return
	}
	return pathToVFSPath(vfsPath)
}
