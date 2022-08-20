//go:build !linux
// +build !linux

package ethash

import (
	"os"
)

// ensureSize expands the file to the given size. This is to prevent runtime
// errors later on, if the underlying file expands beyond the disk capacity,
// even though it ostensibly is already expanded, but due to being sparse
// does not actually occupy the full declared size on disk.
func ensureSize(f *os.File, size int64) error {
	// On systems which do not support fallocate, we merely truncate it.
	// More robust alternatives  would be to
	// - Use posix_fallocate, or
	// - explicitly fill the file with zeroes.
	return f.Truncate(size)
}
