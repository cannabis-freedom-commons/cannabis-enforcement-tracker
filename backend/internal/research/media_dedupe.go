package research

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/frontmatter"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/bits"
	"net/http"
	"os"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	_ "golang.org/x/image/webp"
	"gopkg.in/yaml.v3"
)

// Two photos whose fingerprints differ in at most this many of 64 bits are the same picture: the
// same agency photo republished by several outlets, resized, recompressed or slightly cropped.
const sameImage = 10

// FetchImage downloads a photo through the public-destination-pinned client, bounded in size.
func FetchImage(ctx context.Context, raw string) ([]byte, error) {
	u, err := safeurl.Normalize(raw)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CannabisEnforcementTracker media check)")
	res, err := safeurl.Client().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 15*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 15*1024*1024 {
		return nil, errors.New("image too large")
	}
	return b, nil
}

// fingerprint is a difference hash: the picture shrunk to 9×8 grey cells, one bit per pair of
// neighbouring cells saying which is brighter. Resizing and recompression keep it.
func fingerprint(data []byte) (uint64, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	r := img.Bounds()
	if r.Dx() < 9 || r.Dy() < 8 {
		return 0, errors.New("image too small")
	}
	var cells [8][9]float64
	for y := 0; y < 8; y++ {
		for x := 0; x < 9; x++ {
			x0, x1 := r.Min.X+x*r.Dx()/9, r.Min.X+(x+1)*r.Dx()/9
			y0, y1 := r.Min.Y+y*r.Dy()/8, r.Min.Y+(y+1)*r.Dy()/8
			// sample at most 8×8 points per cell: enough for an average, cheap on large photos
			sx, sy := max(1, (x1-x0)/8), max(1, (y1-y0)/8)
			var sum float64
			var n int
			for py := y0; py < y1; py += sy {
				for px := x0; px < x1; px += sx {
					cr, cg, cb, _ := img.At(px, py).RGBA()
					sum += 0.299*float64(cr) + 0.587*float64(cg) + 0.114*float64(cb)
					n++
				}
			}
			cells[y][x] = sum / float64(n)
		}
	}
	var hash uint64
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			hash <<= 1
			if cells[y][x] > cells[y][x+1] {
				hash |= 1
			}
		}
	}
	return hash, nil
}

// DedupeMedia leaves out photos that repeat an earlier one, or one of the existing report's photos
// (their media addresses in existing): the same address, or the same picture by its fingerprint. A
// photo that cannot be downloaded or decoded is kept, as it cannot be compared. It returns the
// media to keep and the captions of those left out.
func DedupeMedia(ctx context.Context, media []Media, existing []string, fetch func(context.Context, string) ([]byte, error)) ([]Media, []string) {
	seen := map[string]bool{}
	var hashes []uint64
	remember := func(u string) (uint64, bool) {
		if b, err := fetch(ctx, u); err == nil {
			if h, err := fingerprint(b); err == nil {
				return h, true
			}
		}
		return 0, false
	}
	for _, u := range existing {
		seen[u] = true
		if h, ok := remember(u); ok {
			hashes = append(hashes, h)
		}
	}
	kept, dropped := []Media{}, []string{}
	for _, m := range media {
		if m.MediaURL != "" && seen[m.MediaURL] {
			dropped = append(dropped, m.Caption)
			continue
		}
		if m.Kind == "photo" && m.MediaURL != "" && ctx.Err() == nil {
			if h, ok := remember(m.MediaURL); ok {
				same := false
				for _, other := range hashes {
					if bits.OnesCount64(h^other) <= sameImage {
						same = true
						break
					}
				}
				if same {
					dropped = append(dropped, m.Caption)
					continue
				}
				hashes = append(hashes, h)
			}
		}
		if m.MediaURL != "" {
			seen[m.MediaURL] = true
		}
		kept = append(kept, m)
	}
	return kept, dropped
}

// ReportMedia lists the media addresses of an existing report in the checkout.
func ReportMedia(dir, slug string) []string {
	path, err := findReport(dir, slug)
	if err != nil || path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	parts := frontmatter.Split(string(b))
	if len(parts) != 3 {
		return nil
	}
	var header struct {
		Media []struct {
			MediaURL string `yaml:"media_url"`
		} `yaml:"media"`
	}
	if yaml.Unmarshal([]byte(parts[1]), &header) != nil {
		return nil
	}
	out := []string{}
	for _, m := range header.Media {
		if m.MediaURL != "" {
			out = append(out, m.MediaURL)
		}
	}
	return out
}
