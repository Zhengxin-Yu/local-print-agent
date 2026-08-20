//go:build !windows

package render

import "os"

func platformPathUnsafe(string, os.FileInfo) (bool, error) {
	return false, nil
}
