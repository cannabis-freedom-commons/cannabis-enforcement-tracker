// Package archivefile reads private artifacts saved either plain or as Zstandard.
package archivefile

import (
	"errors"
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
)

type reader struct {
	io.Reader
	file    *os.File
	decoder *zstd.Decoder
}

func (r *reader) Close() error {
	if r.decoder != nil {
		r.decoder.Close()
	}
	return r.file.Close()
}

func Open(path string) (io.ReadCloser, error) { return open(os.Open, path) }

// OpenRoot retains os.Root's confinement, including for the compressed fallback.
func OpenRoot(root *os.Root, path string) (io.ReadCloser, error) { return open(root.Open, path) }
func open(opener func(string) (*os.File, error), path string) (io.ReadCloser, error) {
	file, err := opener(path)
	compressed := false
	if os.IsNotExist(err) {
		file, err = opener(path + ".zst")
		compressed = true
	}
	if err != nil {
		return nil, err
	}
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		file.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("not a regular archive file")
	}
	r := &reader{Reader: file, file: file}
	if compressed {
		r.decoder, err = zstd.NewReader(file, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(64<<20))
		if err != nil {
			file.Close()
			return nil, err
		}
		r.Reader = r.decoder
	}
	return r, nil
}
