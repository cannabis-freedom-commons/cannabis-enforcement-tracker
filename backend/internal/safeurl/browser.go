package safeurl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

var ErrBrowserUnavailable = errors.New("Playwright Chromium is unavailable; run npm install and npm run browser-install")

type browserResult struct {
	Evidence    Evidence `json:"evidence"`
	Error       string   `json:"error"`
	Unavailable bool     `json:"unavailable"`
}
type captureBuffer struct {
	bytes.Buffer
	max int
}

func (b *captureBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, errors.New("browser capture output limit")
	}
	return b.Buffer.Write(p)
}

// FetchRendered captures a fresh anonymous browser view. Raw files live only in
// the private artifact directory; there is no connection to the user's browser.
func FetchRendered(parent context.Context, raw, dir, script string) (Evidence, error) {
	normalized, err := Normalize(raw)
	if err != nil {
		return Evidence{}, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return Evidence{}, err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return Evidence{}, err
	}
	script, err = filepath.Abs(script)
	if err != nil {
		return Evidence{}, err
	}
	// Allow time for the browser plus the bounded image compression step.
	ctx, cancel := context.WithTimeout(parent, 75*time.Second)
	defer cancel()
	proxy, token, closeProxy, err := browserProxy(ctx)
	if err != nil {
		return Evidence{}, err
	}
	defer closeProxy()
	home, err := os.MkdirTemp(dir, ".browser-session-")
	if err != nil {
		return Evidence{}, err
	}
	defer os.RemoveAll(home)
	cache := os.Getenv("PLAYWRIGHT_BROWSERS_PATH")
	if cache == "" {
		userHome, _ := os.UserHomeDir()
		if runtime.GOOS == "darwin" {
			cache = filepath.Join(userHome, "Library/Caches/ms-playwright")
		} else {
			cache = filepath.Join(userHome, ".cache/ms-playwright")
			if base := os.Getenv("XDG_CACHE_HOME"); base != "" {
				cache = filepath.Join(base, "ms-playwright")
			}
		}
	}
	command := exec.CommandContext(ctx, "node", script, normalized, dir)
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "TMPDIR=" + home, "PLAYWRIGHT_BROWSERS_PATH=" + cache, "SOURCE_PROXY=" + proxy, "SOURCE_PROXY_TOKEN=" + token}
	// the ad and cookie-notice blocker's switch and engine cache (scripts/source-browser.mjs)
	for _, key := range []string{"SOURCE_ADBLOCK", "SOURCE_BLOCKER_CACHE", "SOURCE_BLOCKER_DEBUG"} {
		if value := os.Getenv(key); value != "" {
			command.Env = append(command.Env, key+"="+value)
		}
	}
	out := &captureBuffer{max: 10 * 1024 * 1024}
	stderr := &captureBuffer{max: 64 * 1024}
	command.Stdout = out
	command.Stderr = stderr
	runErr := command.Run()
	if stderr.Len() > 0 {
		_ = os.WriteFile(filepath.Join(dir, "browser-stderr.log"), stderr.Bytes(), 0600)
	}
	var result browserResult
	if err = json.Unmarshal(out.Bytes(), &result); err != nil {
		if ctx.Err() != nil {
			return Evidence{}, errors.New("browser source capture timed out")
		}
		return Evidence{}, ErrBrowserUnavailable
	}
	result.Evidence.URL = normalized
	result.Evidence.ArtifactDir = dir
	if result.Evidence.FinalURL != "" {
		if _, err = Normalize(result.Evidence.FinalURL); err != nil {
			result.Evidence.FinalURL = normalized
			return result.Evidence, errors.New("the browser could not load the submitted public page")
		}
	}
	if result.Unavailable {
		return result.Evidence, ErrBrowserUnavailable
	}
	if result.Error != "" {
		return result.Evidence, errors.New(result.Error)
	}
	if runErr != nil {
		return result.Evidence, errors.New("browser source capture failed")
	}
	if len(result.Evidence.Content) > 2*1024*1024 || len(result.Evidence.Text) > 2*1024*1024 {
		return Evidence{}, errors.New("rendered source exceeds 2 MB")
	}
	return result.Evidence, nil
}
