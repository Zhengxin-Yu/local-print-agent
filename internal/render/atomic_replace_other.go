//go:build !windows

package render

import "os"

func atomicReplaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
