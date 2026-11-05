package securefs

import (
	"errors"
	"io/fs"
)

func checkOwner(fs.FileInfo) error {
	return errors.New("private-file ownership verification is unsupported on Windows")
}

func checkRootOwner(fs.FileInfo) error {
	return errors.New("approved-root ownership verification is unsupported on Windows")
}
