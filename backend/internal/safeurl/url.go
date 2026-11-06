// Package safeurl applies the same URL policy at intake, redirects and connect time.
package safeurl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

var ErrUnsafeDestination = errors.New("destination blocked")

func PublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001::/23", "2001:db8::/32", "2002::/16", "64:ff9b::/96"} {
		if netip.MustParsePrefix(s).Contains(ip) {
			return false
		}
	}
	return true
}

func Normalize(raw string) (string, error) {
	if len(raw) > 4096 || strings.ContainsAny(raw, "\r\n\t") {
		return "", errors.New("invalid URL")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Opaque != "" {
		return "", errors.New("use a public HTTP(S) news URL")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if !strings.Contains(host, ".") || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return "", errors.New("public hostname required")
	}
	if ip, err := netip.ParseAddr(host); err == nil && !PublicIP(ip) {
		return "", errors.New("private address blocked")
	}
	if p := u.Port(); p != "" && !((u.Scheme == "https" && p == "443") || (u.Scheme == "http" && p == "80")) {
		return "", errors.New("unsupported port")
	}
	u.Host = host
	if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	}
	u.Fragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	q := u.Query()
	for k := range q {
		if strings.HasPrefix(strings.ToLower(k), "utm_") || k == "fbclid" || k == "gclid" {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// dialPublic connects only to hosts whose every address is public, at the checked literal address:
// a second DNS lookup cannot rebind it. Used by the HTTP fetcher and the capture browser's proxy.
func dialPublic(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("no addresses")
	}
	for _, ip := range ips {
		if !PublicIP(ip) {
			return nil, ErrUnsafeDestination
		}
	}
	return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}

func Client() *http.Client {
	tr := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxIdleConns: 16, IdleConnTimeout: 30 * time.Second}
	tr.DialContext = dialPublic
	return &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("redirect limit")
		}
		_, err := Normalize(req.URL.String())
		if err != nil {
			return ErrUnsafeDestination
		}
		return nil
	}}
}

type Evidence struct {
	AccessBlocked  string    `json:"access_blocked,omitempty"`
	ArchiveURL     string    `json:"archive_url,omitempty"`
	ArchivedAt     string    `json:"archived_at,omitempty"`
	CaptureID      string    `json:"capture_id,omitempty"`
	ArtifactDir    string    `json:"-"`
	Text           string    `json:"text,omitempty"`
	Rendered       bool      `json:"rendered,omitempty"`
	Screenshot     string    `json:"screenshot,omitempty"`
	Metadata       *Metadata `json:"metadata,omitempty"`
	BrowserWarning string    `json:"browser_warning,omitempty"`
	Cleanup        *Cleanup  `json:"cleanup,omitempty"`
	URL            string    `json:"url"`
	FinalURL       string    `json:"final_url"`
	Content        string    `json:"content"`
}

// Cleanup is what the browser kept out of a capture: requests the ad and tracker filter lists
// blocked, and what became of a cookie consent dialog (scripts/source-browser.mjs).
type Cleanup struct {
	Blocker         bool   `json:"blocker"`
	BlockedRequests int    `json:"blocked_requests"`
	Consent         string `json:"consent,omitempty"`
}

func Fetch(ctx context.Context, raw string) (Evidence, error) {
	normalized, err := Normalize(raw)
	if err != nil {
		return Evidence{}, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", normalized, nil)
	if err != nil {
		return Evidence{}, err
	}
	req.Header.Set("User-Agent", "CannabisEnforcementTracker/1.0 (source verification)")
	res, err := Client().Do(req)
	if err != nil {
		if errors.Is(err, ErrUnsafeDestination) {
			return Evidence{}, err
		}
		return Evidence{}, fmt.Errorf("the site could not be reached (%w)", err)
	}
	defer res.Body.Close()
	if res.Header.Get("Cf-Mitigated") == "challenge" {
		return Evidence{URL: normalized, FinalURL: res.Request.URL.String(), AccessBlocked: "Cloudflare"}, fmt.Errorf("%w: Cloudflare security verification blocked the article", ErrAccessChallenge)
	}
	// the reasons are written for the public request page: say what went wrong, plainly
	if res.StatusCode != 200 {
		return Evidence{}, fmt.Errorf("the site answered HTTP %d %s", res.StatusCode, http.StatusText(res.StatusCode))
	}
	ct := res.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/") && !strings.Contains(ct, "application/xhtml+xml") {
		kind := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
		if kind == "" {
			kind = "no content type"
		}
		return Evidence{}, fmt.Errorf("it is not a web page (%s)", kind)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024+1))
	if err != nil {
		return Evidence{}, fmt.Errorf("the page could not be read completely (%w)", err)
	}
	if len(b) > 2*1024*1024 {
		return Evidence{}, errors.New("the page is larger than 2 MB")
	}
	return Evidence{URL: normalized, FinalURL: res.Request.URL.String(), Content: string(b)}, nil
}

// StableURLs is useful for deterministic provenance comparisons.
func StableURLs(values []string) []string { sort.Strings(values); return values }

// FetchImage downloads a photo a report links (its media_url) through the public-address-only
// client: an image, at most 25 MB. The address comes from research output, so it gets the same
// destination checks as a source.
func FetchImage(ctx context.Context, raw string) ([]byte, error) {
	normalized, err := Normalize(raw)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", normalized, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "CannabisEnforcementTracker/1.0 (photo thumbnails)")
	req.Header.Set("Accept", "image/avif,image/webp,image/*;q=0.8")
	res, err := Client().Do(req)
	if err != nil {
		if errors.Is(err, ErrUnsafeDestination) {
			return nil, err
		}
		return nil, errors.New("fetch failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if ct := strings.TrimSpace(strings.SplitN(res.Header.Get("Content-Type"), ";", 2)[0]); ct != "" && !strings.HasPrefix(ct, "image/") && ct != "application/octet-stream" {
		return nil, fmt.Errorf("not an image (%s)", ct)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 25*1024*1024+1))
	if err != nil {
		return nil, errors.New("fetch failed")
	}
	if len(b) > 25*1024*1024 {
		return nil, errors.New("larger than 25 MB")
	}
	return b, nil
}

// FetchVideoStart downloads the beginning of a video file a report links, at most max bytes,
// through the public-address-only client: enough for a frame when the file is laid out for
// streaming. Only video (or unlabelled) content is accepted.
func FetchVideoStart(ctx context.Context, raw string, max int64) ([]byte, error) {
	normalized, err := Normalize(raw)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", normalized, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "CannabisEnforcementTracker/1.0 (video posters)")
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", max-1))
	res, err := Client().Do(req)
	if err != nil {
		if errors.Is(err, ErrUnsafeDestination) {
			return nil, err
		}
		return nil, errors.New("fetch failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if ct := strings.TrimSpace(strings.SplitN(res.Header.Get("Content-Type"), ";", 2)[0]); ct != "" && !strings.HasPrefix(ct, "video/") && ct != "application/octet-stream" && ct != "binary/octet-stream" {
		return nil, fmt.Errorf("not a video (%s)", ct)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, max))
	if err != nil && len(b) == 0 {
		return nil, errors.New("fetch failed")
	}
	return b, nil
}
