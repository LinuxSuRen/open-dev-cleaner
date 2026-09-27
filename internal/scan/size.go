package scan

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DirSize walks path and returns (bytes, fileCount) of regular files.
// Symlinked directories are not followed. Unreadable entries are skipped.
func DirSize(ctx context.Context, path string) (int64, int64) {
	if path == "" {
		return 0, 0
	}
	var total, files int64
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		total += info.Size()
		files++
		if files%4096 == 0 { // cooperative cancellation
			select {
			case <-ctx.Done():
				return fs.SkipAll
			default:
			}
		}
		return nil
	})
	return total, files
}

// FormatSize renders bytes as a human readable binary string.
func FormatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ParseSize parses docker-style human sizes ("382B", "1.24kB", "5.6GB",
// "12 MB", "2GiB"). It is deliberately tolerant and returns 0 on failure.
func ParseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Take the leading numeric part.
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == ',') {
		i++
	}
	numPart, unitPart := s[:i], strings.TrimSpace(s[i:])
	numPart = strings.ReplaceAll(numPart, ",", "")
	if numPart == "" || numPart == "." {
		return 0
	}
	val, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0
	}
	unit := strings.ToLower(unitPart)
	// Docker prints kB/MB/GB (decimal-ish); binary KiB variants too. We use
	// 1024 for everything: estimates only need to be roughly right.
	mult := int64(1)
	switch {
	case strings.HasPrefix(unit, "k"):
		mult = 1024
	case strings.HasPrefix(unit, "m"):
		mult = 1024 * 1024
	case strings.HasPrefix(unit, "g"):
		mult = 1024 * 1024 * 1024
	case strings.HasPrefix(unit, "t"):
		mult = 1024 * 1024 * 1024 * 1024
	case strings.HasPrefix(unit, "p"):
		mult = 1024 * 1024 * 1024 * 1024 * 1024
	}
	return int64(val * float64(mult))
}

// ModTime returns the modification time of path or the zero time.
func ModTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}
