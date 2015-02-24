// +build !windows

package cacheddownloader

import "os"

func Replace(src, dst string) error {
     return os.Rename(src, dst)
}