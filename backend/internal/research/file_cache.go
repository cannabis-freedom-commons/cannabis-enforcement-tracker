package research

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Reference data (countries, vocabularies, agencies) is read for every report checked: thousands
// of times when the whole repository is checked. It is parsed once and reused until one of its
// files changes (modification time or size). Callers only read the returned maps.
var fileCache sync.Map // key → cachedFiles

type cachedFiles struct {
	stamp string
	value any
	err   error
}

func filesStamp(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%d:%d;", info.ModTime().UnixNano(), info.Size())
		} else {
			b.WriteString("missing;")
		}
	}
	return b.String()
}

func cachedLoad[T any](kind string, paths []string, load func() (T, error)) (T, error) {
	for i, p := range paths {
		if abs, err := filepath.Abs(p); err == nil {
			paths[i] = abs
		}
	}
	key := kind + "\x00" + strings.Join(paths, "\x00")
	stamp := filesStamp(paths)
	if c, ok := fileCache.Load(key); ok && c.(cachedFiles).stamp == stamp {
		return c.(cachedFiles).value.(T), c.(cachedFiles).err
	}
	value, err := load()
	fileCache.Store(key, cachedFiles{stamp: stamp, value: value, err: err})
	return value, err
}
