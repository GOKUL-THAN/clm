package securefs

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func checkOwner(info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot verify private-file ownership")
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("private file is not owned by the gateway process user")
	}
	return nil
}

func checkRootOwner(info fs.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot verify approved-root ownership")
	}
	if int(stat.Uid) != 0 && int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("approved root must be owned by root or the gateway process user")
	}
	return nil
}
