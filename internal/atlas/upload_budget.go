package atlas

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type photoBudget struct {
	mu          sync.Mutex
	used, limit int64
	ready       bool
}

func newPhotoBudget(directory string, limit int64) *photoBudget {
	b := &photoBudget{limit: limit, ready: true}
	err := filepath.WalkDir(filepath.Join(directory, "runtime", "uploads"), func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		b.used += info.Size()
		return nil
	})
	b.ready = err == nil
	return b
}
func (b *photoBudget) reserve(bytes int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.ready || bytes > b.limit-b.used {
		return false
	}
	b.used += bytes
	return true
}
func (b *photoBudget) release(bytes int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.used = max(0, b.used-bytes)
}

// An encoder must not turn a small compressed PNG into an unbounded JPEG file.
type cappedImageWriter struct {
	file      *os.File
	remaining int64
}

func (w *cappedImageWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errors.New("照片编码后超过 5 MB，请压缩图片")
	}
	n, err := w.file.Write(p)
	w.remaining -= int64(n)
	return n, err
}
