package research

import (
	"github.com/klauspost/compress/zstd"
	"os"
	"path/filepath"
	"testing"
)

// A run's tokens counted again from its log include reasoning and cached input, which were missed.
func TestCountTokensFromLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "research-events.jsonl")
	log := `{"type":"step_finish","part":{"type":"step-finish","cost":0,"tokens":{"input":3897,"output":173,"reasoning":0,"cache":{"read":0,"write":0}}}}
{"type":"text","part":{"text":"hello"}}
{"type":"step_finish","part":{"type":"step-finish","cost":0,"tokens":{"input":6187,"output":98,"reasoning":10,"cache":{"read":3840,"write":5}}}}
not json
`
	if err := os.WriteFile(path, []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	got, ok := countTokens(path)
	if !ok || got != [5]int64{10084, 271, 10, 3840, 5} {
		t.Fatal(got, ok)
	}
	encoder, _ := zstd.NewWriter(nil)
	os.WriteFile(path+".zst", encoder.EncodeAll([]byte(log), nil), 0600)
	encoder.Close()
	os.Remove(path)
	compressed, ok := countTokens(path)
	if !ok || compressed != got {
		t.Fatal("compressed token log differs", compressed, got)
	}
	if _, ok = countTokens(filepath.Join(t.TempDir(), "missing.jsonl")); ok {
		t.Fatal("missing log counted")
	}
}
