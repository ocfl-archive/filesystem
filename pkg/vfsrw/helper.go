package vfsrw

import (
	"regexp"

	"emperror.dev/errors"
)

var matchPathRegexp = regexp.MustCompile(`^vfs://?([^/]+)/(.*)$`)

func matchPath(vfsPath string) (name string, path string, err error) {
	matches := matchPathRegexp.FindStringSubmatch(vfsPath)
	if matches == nil {
		err = errors.Errorf("invalid path format '%s'", vfsPath)
		return
	}
	name = matches[1]
	path = matches[2]
	return
}
