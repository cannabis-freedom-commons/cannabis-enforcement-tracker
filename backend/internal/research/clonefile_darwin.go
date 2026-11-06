package research

import "golang.org/x/sys/unix"

// cloneFile makes an APFS copy-on-write clone; the target must not exist.
func cloneFile(source, target string) error {
	return unix.Clonefile(source, target, unix.CLONE_NOFOLLOW)
}
