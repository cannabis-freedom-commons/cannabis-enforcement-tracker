package store

import (
	"context"
	"path/filepath"
	"testing"
)

// The public pages read through a read-only pool beside the one writer, so a long write does not
// queue their queries; the pool sees committed writes and cannot write itself.
func TestReadPoolSeesWritesAndCannotWrite(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Read == nil {
		t.Fatal("no read pool for a file database")
	}
	ctx := context.Background()
	before, err := s.FeedVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec("UPDATE submission_feed_version SET version=version+1 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	if after, err := s.FeedVersion(ctx); err != nil || after == before {
		t.Fatalf("the read pool did not see the write: %s → %s (%v)", before, after, err)
	}
	if _, err = s.Read.Exec("UPDATE submission_feed_version SET version=0"); err == nil {
		t.Fatal("the read pool could write")
	}
	// a write held open does not block a read
	tx, err := s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE submission_feed_version SET version=version+1 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListSubmissions(ctx, "", "", "", "updated", "desc"); err != nil {
		t.Fatal("a read during a write:", err)
	}
	if mem, err := Open(":memory:"); err != nil || mem.Read != nil {
		t.Fatal("an in-memory database has one connection only", err)
	}
}
