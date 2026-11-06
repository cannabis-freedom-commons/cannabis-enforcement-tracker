//go:build !darwin

package research

import "errors"

func cloneFile(source, target string) error { return errors.New("no copy-on-write clone") }
