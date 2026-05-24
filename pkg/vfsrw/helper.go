package vfsrw

import (
	"path"
	"regexp"
	"strings"

	"emperror.dev/errors"
)

var matchPathRegexp = regexp.MustCompile(`^vfs://?([^/]+)(/(.*))?$`)

func MatchPath(vfsPath string) (vPath, name string, pathStr string, err error) {
	if strings.HasPrefix(vfsPath, "vfs:/") {
		matches := matchPathRegexp.FindStringSubmatch(vfsPath)
		if matches == nil {
			return "", "", "", errors.Errorf("invalid vfs path: %s", vfsPath)
		}
		name = matches[1]
		pathStr = matches[3]
		vPath = path.Join("vfs:/", name, pathStr)
		return
	}
	return pathToVFSPath(vfsPath)
}
