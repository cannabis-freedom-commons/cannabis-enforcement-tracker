package worker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/research"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
)

// resumableWorkspace returns the case's workspace whose interrupted OpenCode session this worker's
// model can continue, discarding every other kept session of the case. "" means start fresh.
func (w *Worker) resumableWorkspace(cid string) string {
	root, err := filepath.Abs(w.Config.Workspaces)
	if err != nil {
		return ""
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	type candidate struct {
		dir     string
		modTime int64
	}
	var kept []candidate
	for _, e := range entries {
		m := workspaceName.FindStringSubmatch(e.Name())
		if !e.IsDir() || m == nil || m[1] != cid {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if !research.HasSession(dir) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		kept = append(kept, candidate{dir, info.ModTime().UnixNano()})
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].modTime > kept[j].modTime })
	chosen := ""
	for _, c := range kept {
		if chosen == "" && research.CanResume(c.dir, w.Config.ResearchModel) {
			chosen = c.dir
			continue
		}
		research.DiscardSession(c.dir)
	}
	return chosen
}

// resultCheck is the agent's check_result tool: the service's own checks that need no network,
// run on a proposal before the agent ends its run. Quotations are looked up only in sources
// already captured; the final verification still opens the others.
func (w *Worker) resultCheck(dir string, initial safeurl.Evidence, base []safeurl.Evidence, addedAt, code string) research.CheckFunc {
	return func(ctx context.Context, r research.Result) []string {
		b, err := os.ReadFile(filepath.Join(w.Config.RepoPath, "src/data/countries.json"))
		if err != nil {
			return nil // the service checks again after the run
		}
		var countries []struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(b, &countries) != nil {
			return nil
		}
		ids := map[string]bool{}
		for _, c := range countries {
			ids[c.ID] = true
		}
		r.CreatedByModel = w.Config.ResearchModel
		r.CleanPresentation()
		r.Tags = research.CleanTags(r.Tags)
		var vocabs []research.Vocabularies
		if vocab, err := research.LoadVocabularies(w.Config.RepoPath); err == nil {
			vocabs = append(vocabs, vocab)
		}
		if err := r.Validate(ids, vocabs...); err != nil {
			return []string{"schema validation: " + err.Error()}
		}
		if !r.Drafts() {
			return nil
		}
		if r.Outcome == "new_event" {
			r.AddedAt, r.Code = addedAt, code
		}
		var problems []string
		notCaptured := func(context.Context, string) (safeurl.Evidence, error) {
			return safeurl.Evidence{}, errors.New("not captured yet")
		}
		evidence := append(append([]safeurl.Evidence(nil), base...), research.ReadToolEvidence(dir)...)
		_, _, check, err := r.Confirm(ctx, initial, notCaptured, evidence...)
		for _, field := range r.MissingQuotes() {
			problems = append(problems, field+" has no data_quotes entry: add a verbatim quotation from a cited source that shows this value, or leave the value out")
		}
		for _, field := range check.DataLeftOut {
			problems = append(problems, field+" would be left out: its quotation is not in the captured text of its source, or that source is not cited by any fact")
		}
		for _, text := range check.LeftOut {
			problems = append(problems, "this fact's quotation is not in the captured text of its source, so it would be left out (quote the source exactly): "+text)
		}
		if err != nil && check.Checked > 0 {
			problems = append(problems, "quotation check: "+err.Error())
		}
		if r.Outcome == "new_event" {
			reports, err := Coverage(dir)
			if err == nil {
				if err = research.CheckEventComparisons(r, reports, false); err != nil {
					problems = append(problems, "event identity: "+err.Error()+". Read each report with tracker_get_report (or existing-reports/<slug>.md) and give event_comparisons for all of them at once; tracker_find_reports with your result lists every report this check will require. A matching event or later case stage must update the existing report instead")
				}
			}
			if err = research.CheckRetiredIdentity(dir, r.Slug, r.Code); err != nil {
				problems = append(problems, "event identity: "+err.Error())
			}
		}
		draft, err := research.PrepareDraft(dir, r)
		if errors.Is(err, research.ErrUnchanged) {
			return problems
		}
		if errors.Is(err, research.ErrMergeTarget) {
			problems = append(problems, "update: "+err.Error()+". Copy each corroborated sentence of the current target report exactly into existing_text, without citations; only genuinely new details may use new_detail:true")
		} else if errors.Is(err, research.ErrOpenQuestions) {
			problems = append(problems, "update: "+err.Error()+". Send open_questions with the report's complete revised list: one plain sentence for each real gap or disagreement about the case, answered and repeated ones merged or dropped, no notes about sources, pages or media")
		} else if err != nil {
			problems = append(problems, "draft: "+err.Error())
		} else {
			for _, f := range append([]research.Draft{draft}, draft.Updates...) {
				if f.Content == "" {
					continue
				}
				if err := research.CheckReport(dir, f.Path, f.Content); err != nil {
					problems = append(problems, "report check for "+f.Path+": "+err.Error())
				}
			}
		}
		return problems
	}
}
