package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/frontmatter"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"gopkg.in/yaml.v3"
)

// runEnrich adds structured details to existing reports, newest first: it opens each report's
// sources, asks the research agent for quoted values, checks every quotation and writes the
// report. Conflicts with the report's own figures and left-out values go to a review file.
func runEnrich(ctx context.Context, c config.Config, limit int, only string, w io.Writer) error {
	agents := c.Agents()
	if len(agents) == 0 {
		return errors.New("configure RESEARCH_AGENTS or RESEARCH_MODEL first")
	}
	c.ResearchModel = agents[0].Model
	var paths []string
	err := filepath.WalkDir(filepath.Join(c.RepoPath, "reports"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		if only != "" && strings.TrimSuffix(d.Name(), ".md") != only {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(paths)))
	audits := filepath.Join(filepath.Dir(c.Database), "audits")
	if err = os.MkdirAll(audits, 0700); err != nil {
		return err
	}
	review, err := os.OpenFile(filepath.Join(audits, "enrich-"+time.Now().Format("2006-01-02")+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer review.Close()
	runner := research.Runner{Config: c}
	done := 0
	for _, path := range paths {
		if done >= limit || ctx.Err() != nil {
			break
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		parts := frontmatter.Split(string(b))
		if len(parts) != 3 {
			continue
		}
		var data struct {
			Details any `yaml:"details"`
			Sources []struct {
				URL string `yaml:"url"`
			} `yaml:"sources"`
		}
		if yaml.Unmarshal([]byte(parts[1]), &data) != nil || data.Details != nil {
			continue
		}
		rel, _ := filepath.Rel(c.RepoPath, path)
		opened := map[int]safeurl.Evidence{}
		for i, s := range data.Sources {
			if s.URL == "" {
				continue
			}
			fetchCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			e, err := safeurl.ResolveSource(fetchCtx, s.URL, safeurl.Fetch, safeurl.LookupArchive)
			cancel()
			if err == nil && e.Content != "" && safeurl.AccessChallenge(e) == "" {
				opened[i+1] = e
			}
		}
		done++
		if len(opened) == 0 {
			fmt.Fprintf(w, "%s: no source could be opened, skipped\n", rel)
			continue
		}
		runCtx, cancel := context.WithTimeout(ctx, c.ResearchTimeout)
		result, err := runner.Enrich(runCtx, c.RepoPath, rel, opened)
		cancel()
		entry := map[string]any{"report": rel, "added": result.Added, "left_out": result.LeftOut, "conflicts": result.Conflicts, "model": c.ResearchModel, "at": time.Now().UTC().Format(time.RFC3339)}
		if err != nil {
			entry["error"] = err.Error()
			fmt.Fprintf(w, "%s: %v\n", rel, err)
		} else if result.Content != "" {
			if err = os.WriteFile(path, []byte(result.Content), 0644); err != nil {
				return err
			}
			fmt.Fprintf(w, "%s: added %s\n", rel, strings.Join(result.Added, ", "))
		} else {
			fmt.Fprintf(w, "%s: nothing supported to add\n", rel)
		}
		if len(result.Conflicts) > 0 {
			fmt.Fprintf(w, "  conflicts for review: %s\n", strings.Join(result.Conflicts, "; "))
		}
		line, _ := json.Marshal(entry)
		if _, err = review.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	fmt.Fprintf(w, "Checked %d reports. Review notes: %s\n", done, review.Name())
	return nil
}
