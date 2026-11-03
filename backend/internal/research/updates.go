package research

import (
	"errors"
	"fmt"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/frontmatter"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/reportcode"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"gopkg.in/yaml.v3"
)

// ReportUpdate changes another existing report in the same run: it adds this run's sources to it
// (another account of an event the tracker already records, or a related report the article also
// covers) and can add open questions and correct its title or summary. Each fact names a source of
// this run and quotes it; its text is the report's sentence it supports, which then cites the
// added source too. Explicit new_detail facts add novel sentences. Nothing else changes.
type ReportUpdate struct {
	Report        string   `json:"report"`
	Facts         []Fact   `json:"facts"`
	Uncertainties []string `json:"uncertainties,omitempty"`
	Title         string   `json:"title,omitempty"`
	Summary       string   `json:"summary,omitempty"`
}

// ErrUnchanged is an update that would change nothing: its sources are already in the report.
var ErrUnchanged = errors.New("the report already cites these sources")

func (u ReportUpdate) validate(sources int, cited map[int]bool) error {
	if !slug.MatchString(u.Report) || len(u.Report) > 100 {
		return errors.New("invalid report to update")
	}
	if len(u.Facts) == 0 || len(u.Facts) > 10 || len(u.Uncertainties) > 10 {
		return errors.New("invalid report update")
	}
	for _, f := range u.Facts {
		if err := validateMergeFact(f); err != nil {
			return err
		}
		if f.Source < 1 || f.Source > sources || len(f.Text) < 10 || len(f.Text) > 2000 || len(f.Quote) < 20 || len(f.Quote) > 4000 || plain.MatchString(f.Text) {
			return errors.New("invalid report update fact")
		}
		if cited != nil && !cited[f.Source] {
			return errors.New("a report update may only use sources the answer cites")
		}
	}
	for _, q := range u.Uncertainties {
		if strings.TrimSpace(q) == "" || len(q) > 4000 || strings.ContainsAny(q, "\x00") {
			return errors.New("invalid open question")
		}
	}
	if len(u.Title) > 120 || len(u.Summary) > 400 || plain.MatchString(u.Title) || plain.MatchString(u.Summary) {
		return errors.New("invalid corrected title or summary")
	}
	return nil
}

// Primary is the update an additional_source answer makes to its existing report.
func (r Result) Primary() ReportUpdate {
	return ReportUpdate{Report: r.ExistingReport, Facts: r.Facts, Uncertainties: r.Uncertainties}
}

func findReport(dir, name string) (string, error) {
	var found string
	err := filepath.WalkDir(filepath.Join(dir, "reports"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == name+".md" {
			if found != "" {
				return errors.New("ambiguous existing slug")
			}
			found = path
		}
		return nil
	})
	return found, err
}

// mapping is the value under key in a YAML mapping node, or nil.
func mapping(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func scalar(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }

var citation = regexp.MustCompile(`^[\s.,;:!?"'’”)]*\[@([0-9, ]+)\]`)

// cite adds source n to the citation that follows sentence in body; false if the sentence is not there.
func cite(body, sentence string, n int) (string, bool) {
	sentence = strings.TrimRight(strings.TrimSpace(sentence), ".")
	if sentence == "" {
		return body, false
	}
	words := strings.Fields(sentence)
	for i := range words {
		words[i] = regexp.QuoteMeta(words[i])
	}
	pattern := regexp.MustCompile(strings.Join(words, `\s+`))
	for _, match := range pattern.FindAllStringIndex(body, -1) {
		at := match[1]
		if m := citation.FindStringSubmatchIndex(body[at:]); m != nil {
			nums := map[int]bool{n: true}
			for _, part := range strings.Split(body[at+m[2]:at+m[3]], ",") {
				if k, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
					nums[k] = true
				}
			}
			list := []int{}
			for k := range nums {
				list = append(list, k)
			}
			sort.Ints(list)
			parts := make([]string, len(list))
			for j, k := range list {
				parts[j] = strconv.Itoa(k)
			}
			return body[:at+m[2]] + strings.Join(parts, ",") + body[at+m[3]:], true
		}
	}
	return body, false
}

// UpdateReport applies an update to an existing report in the checkout, keeping the rest of the
// file as it is.
func UpdateReport(dir string, u ReportUpdate, sources []Source, media []Media) (Draft, error) {
	return mergeReport(dir, u, sources, media, nil)
}

func mergeReport(dir string, u ReportUpdate, sources []Source, media []Media, current *Result) (Draft, error) {
	path, err := findReport(dir, u.Report)
	if err != nil {
		return Draft{}, err
	}
	if path == "" {
		return Draft{}, fmt.Errorf("report to update not found: %s", u.Report)
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return Draft{}, err
	}
	d := Draft{Path: filepath.ToSlash(rel), PreviousPath: filepath.ToSlash(rel), Slug: u.Report}
	b, err := os.ReadFile(path)
	if err != nil {
		return d, err
	}
	parts := frontmatter.Split(string(b))
	if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
		return d, errors.New("invalid existing report")
	}
	var doc yaml.Node
	if err = yaml.Unmarshal([]byte(parts[1]), &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return d, errors.New("invalid existing report front matter")
	}
	header := doc.Content[0]
	if title := mapping(header, "title"); title != nil {
		d.Title = title.Value
	}
	list := mapping(header, "sources")
	if list == nil || list.Kind != yaml.SequenceNode || len(list.Content) == 0 {
		return d, errors.New("invalid existing sources")
	}
	front := strings.Split(strings.Trim(parts[1], "\n"), "\n")
	// Repair legacy JSON front matter, then preserve normal YAML line-for-line.
	if strings.HasPrefix(strings.TrimSpace(parts[1]), "{") {
		clearYAMLStyle(header)
		encoded, err := yaml.Marshal(header)
		if err != nil {
			return d, err
		}
		front = strings.Split(strings.TrimRight(string(encoded), "\n"), "\n")
	}
	// Legacy reports gain a code on update; existing codes survive every case change.
	codeAdded := mapping(header, "code") == nil
	if codeAdded {
		used, err := reportcode.Index(dir)
		if err != nil {
			return d, err
		}
		front = append([]string{"code: " + strconv.Quote(reportcode.Available(u.Report, used))}, front...)
	}
	// this run's sources, numbered as in the report: an already cited one keeps its number
	number := map[int]int{}
	added := map[string]bool{}
	var newSources, newMedia []*yaml.Node
	archivesChanged := false
	pendingSources := map[string]int{}
	for _, f := range u.Facts {
		if err := validateMergeFact(f); err != nil {
			return d, err
		}
		if f.Source < 1 || f.Source > len(sources) {
			return d, errors.New("invalid merge source")
		}
		if number[f.Source] != 0 {
			continue
		}
		s := sources[f.Source-1]
		normalized, err := safeurl.Normalize(s.URL)
		if err != nil {
			return d, err
		}
		if n := pendingSources[normalized]; n != 0 {
			number[f.Source] = n
			continue
		}
		for j, old := range list.Content {
			if url := mapping(old, "url"); url != nil {
				if known, _ := safeurl.Normalize(url.Value); known == normalized {
					number[f.Source] = j + 1
					if s.Archived != "" && (mapping(old, "archived") == nil || mapping(old, "archived").Value != s.Archived) {
						setMapping(old, "archived", scalar(s.Archived))
						archivesChanged = true
						// Re-encode just this list when adding archive provenance.
					}
					break
				}
			}
		}
		if number[f.Source] == 0 {
			newSources = append(newSources, entry([][2]string{{"title", s.Title}, {"url", s.URL}, {"publisher", s.Publisher}, {"date", s.Date}, {"language", s.Language}, {"archived", s.Archived}}))
			number[f.Source] = len(list.Content) + len(newSources)
			added[normalized] = true
		}
		pendingSources[normalized] = number[f.Source]
	}
	if archivesChanged {
		front, err = setNode(front, "sources", list)
		if err != nil {
			return d, err
		}
	}
	changed := len(newSources) > 0 || archivesChanged || codeAdded
	body := strings.TrimSpace(parts[2])
	extra := ""
	for _, f := range u.Facts {
		n := number[f.Source]
		target := f.Text
		if f.ExistingText != "" {
			target = f.ExistingText
		}
		next, ok := cite(body, target, n)
		if ok {
			changed = changed || next != body
			body = next
		} else if next, ok := cite(extra, target, n); ok {
			changed = changed || extra != next
			extra = next
		} else if f.ExistingText != "" || !f.NewDetail {
			return d, errors.New("merge fact must match an existing cited sentence or explicitly be a new_detail; do not append a paraphrase of existing prose")
		} else if !strings.Contains(body, strings.TrimSpace(f.Text)) {
			if extra != "" {
				extra += " "
			}
			extra += strings.TrimSpace(f.Text) + " [@" + strconv.Itoa(n) + "]"
			changed = true
		}
	}
	if extra != "" {
		body += "\n\n" + extra
	}
	var oldMedia []*yaml.Node
	if items := mapping(header, "media"); items != nil {
		oldMedia = items.Content
	}
	for _, m := range media {
		if normalized, _ := safeurl.Normalize(m.URL); current == nil && !added[normalized] {
			continue // only the photos of the sources this update adds
		}
		seen := false
		for _, old := range append(oldMedia, newMedia...) {
			if v := mapping(old, "media_url"); v != nil && v.Value == m.MediaURL {
				seen = true
			}
		}
		if !seen {
			newMedia = append(newMedia, entry([][2]string{{"kind", m.Kind}, {"url", m.URL}, {"media_url", m.MediaURL}, {"publisher", m.Publisher}, {"caption", strings.Join(strings.Fields(m.Caption), " ")}}))
			changed = true
		}
	}
	sets := [][2]string{}
	if current != nil {
		front, err = mergeCaseFields(front, header, *current)
		if err != nil {
			return d, err
		}
		changed = changed || strings.Join(front, "\n") != strings.Trim(parts[1], "\n")
	}
	for _, kv := range [][2]string{{"title", u.Title}, {"summary", u.Summary}} {
		if old := mapping(header, kv[0]); kv[1] != "" && (old == nil || old.Value != kv[1]) {
			sets = append(sets, kv)
		}
	}
	corrected := len(sets) > 0
	if corrected {
		changed = true
		if u.Title != "" {
			d.Title = u.Title
		}
	}
	var newQuestions []*yaml.Node
	{
		var old []*yaml.Node
		if questions := mapping(header, "open_questions"); questions != nil {
			old = questions.Content
		}
		for _, q := range u.Uncertainties {
			seen := false
			for _, o := range append(old, newQuestions...) {
				seen = seen || o.Value == q
			}
			if !seen {
				newQuestions = append(newQuestions, scalar(q))
				changed = true
			}
		}
	}
	if !changed {
		return d, ErrUnchanged
	}
	// Preserve the verification of earlier prose only when the new facts are checked.
	// A checked addition cannot certify unchecked older prose or a corrected summary.
	previousStatus := ""
	if status := mapping(header, "status"); status != nil {
		previousStatus = status.Value
	}
	nextStatus := updateStatus(previousStatus, factsVerified(u.Facts) && !corrected)
	if nextStatus != previousStatus {
		sets = append(sets, [2]string{"status", nextStatus})
	}
	for _, add := range []struct {
		key   string
		items []*yaml.Node
	}{{"sources", newSources}, {"media", newMedia}, {"open_questions", newQuestions}} {
		if front, err = appendItems(front, add.key, add.items); err != nil {
			return d, err
		}
	}
	for _, kv := range sets {
		if front, err = setValue(front, kv[0], kv[1]); err != nil {
			return d, err
		}
	}
	var check yaml.Node
	if err = yaml.Unmarshal([]byte(strings.Join(front, "\n")), &check); err != nil {
		return d, fmt.Errorf("updated front matter is invalid: %w", err)
	}
	if title := mapping(check.Content[0], "title"); title != nil {
		d.Title = title.Value
	}
	d.Content = "---\n" + strings.Join(front, "\n") + "\n---\n\n" + body + "\n"
	return d, nil
}

// entry is a YAML mapping of the non-empty pairs, in order; a date stays an unquoted date.
func entry(pairs [][2]string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, kv := range pairs {
		if kv[1] == "" {
			continue
		}
		value := scalar(kv[1])
		if kv[0] == "date" {
			value.Tag = "!!timestamp"
		}
		n.Content = append(n.Content, scalar(kv[0]), value)
	}
	return n
}

// block finds the front-matter lines of a top-level key: its own line and the indented or list
// lines under it.
func block(front []string, key string) (start, end int) {
	start = -1
	for i, line := range front {
		if start < 0 {
			if strings.HasPrefix(line, key+":") {
				start = i
			}
			continue
		}
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "-") {
			return start, i
		}
	}
	if start < 0 {
		return -1, -1
	}
	return start, len(front)
}

// appendItems adds list items under a top-level key, after its last item, in the list's own
// indentation; the key is added at the end when the report has none.
func appendItems(front []string, key string, items []*yaml.Node) ([]string, error) {
	if len(items) == 0 {
		return front, nil
	}
	b, err := yaml.Marshal(&yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items})
	if err != nil {
		return front, err
	}
	start, end := block(front, key)
	indent := "  "
	if start >= 0 {
		if strings.TrimSpace(front[start]) != key+":" {
			return front, fmt.Errorf("cannot add to the %s list in this layout", key)
		}
		for _, line := range front[start+1 : end] {
			if trimmed := strings.TrimLeft(line, " "); strings.HasPrefix(trimmed, "- ") {
				indent = line[:len(line)-len(trimmed)]
				break
			}
		}
		for end > start+1 && strings.TrimSpace(front[end-1]) == "" {
			end--
		}
	} else {
		front = append(front, key+":")
		start, end = len(front)-1, len(front)
	}
	lines := []string{}
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		lines = append(lines, indent+line)
	}
	return append(front[:end], append(lines, front[end:]...)...), nil
}

// setValue replaces a top-level key's value, or adds the key at the end.
func setValue(front []string, key, value string) ([]string, error) {
	b, err := yaml.Marshal(&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{scalar(key), scalar(value)}})
	if err != nil {
		return front, err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	start, end := block(front, key)
	if start < 0 {
		return append(front, lines...), nil
	}
	return append(front[:start], append(lines, front[end:]...)...), nil
}
