package research

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func validateMergeFact(f Fact, i int) error {
	if p := mergeProblem(f); p != "" {
		return fmt.Errorf("fact %d (%q): %s", i+1, excerpt(f.Text), p)
	}
	return nil
}

func existingReportFiles(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(filepath.Join(dir, "reports"), func(path string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("report snapshot must contain regular files")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := "existing-reports/" + entry.Name()
		if _, exists := files[name]; exists {
			return errors.New("ambiguous report slug in research snapshot")
		}
		files[name] = b
		return nil
	})
	return files, err
}

func updateCaseReport(dir string, r Result) (Draft, error) {
	if r.ExistingReport != r.Slug {
		return Draft{}, errors.New("existing report slug differs")
	}
	d, err := mergeReport(dir, r.Primary(), r.Sources, r.Media, &r)
	if err != nil {
		return d, err
	}
	d.Path = filepath.ToSlash(filepath.Join("reports", r.Date[:4], r.Date[5:7], r.Slug+".md"))
	return d, nil
}

func clearYAMLStyle(node *yaml.Node) {
	node.Style = 0
	for _, child := range node.Content {
		clearYAMLStyle(child)
	}
}

func setMapping(node *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1] = value
			return
		}
	}
	node.Content = append(node.Content, scalar(key), value)
}

func setNode(front []string, key string, value *yaml.Node) ([]string, error) {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{scalar(key), value}}
	b, err := yaml.Marshal(n)
	if err != nil {
		return front, err
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	start, end := block(front, key)
	if start < 0 {
		return append(front, lines...), nil
	}
	out := append([]string{}, front[:start]...)
	out = append(out, lines...)
	return append(out, front[end:]...), nil
}

func mergeCaseFields(front []string, header *yaml.Node, r Result, number map[int]int) ([]string, error) {
	country, date := mapping(header, "country"), mapping(header, "date")
	if country == nil || country.Value != r.Country {
		return front, errors.New("existing report country differs")
	}
	if date == nil || len(date.Value) < 10 {
		return front, errors.New("invalid existing date")
	}
	if r.Date < date.Value[:10] {
		return front, errors.New("earlier coverage requires editorial consolidation")
	}
	changes := entry(nil)
	oldType := mapping(header, "type")
	// Another outlet describing the same step is corroboration, even if the model
	// selected existing_case. Its alternate headline/summary must not replace ours.
	if r.Date != date.Value[:10] || oldType == nil || r.Type != oldType.Value {
		changes = entry([][2]string{{"date", r.Date}, {"title", r.Title}, {"summary", r.Summary}, {"type", r.Type}})
	}
	if creator := mapping(header, "created_by_model"); creator == nil || strings.TrimSpace(creator.Value) == "" {
		setMapping(changes, "created_by_model", scalar("unknown"))
	}
	if start, end := block(front, "updated_by_models"); start >= 0 {
		front = append(append([]string{}, front[:start]...), front[end:]...)
	}
	also := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
	seen := map[string]bool{r.Type: true}
	addType := func(t string) {
		if t != "" && !seen[t] {
			also.Content = append(also.Content, scalar(t))
			seen[t] = true
		}
	}
	if old := mapping(header, "also"); old != nil {
		for _, v := range old.Content {
			addType(v.Value)
		}
	}
	if old := mapping(header, "type"); old != nil {
		addType(old.Value)
	}
	for _, t := range r.Also {
		addType(t)
	}
	if len(also.Content) > 0 || mapping(header, "also") != nil {
		setMapping(changes, "also", also)
	}
	if r.Figures != nil {
		// JSON field tags are the schema's snake_case names; convert to a YAML node.
		b, _ := json.Marshal(r.Figures)
		var proposed yaml.Node
		if err := yaml.Unmarshal(b, &proposed); err != nil {
			return front, err
		}
		figures := mapping(header, "figures")
		if figures == nil {
			figures = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		for i := 0; i+1 < len(proposed.Content[0].Content); i += 2 {
			setMapping(figures, proposed.Content[0].Content[i].Value, proposed.Content[0].Content[i+1])
		}
		clearYAMLStyle(figures)
		setMapping(changes, "figures", figures)
	}
	// a later stage replaces the details it gives (case stage, sentence types…) and keeps the rest
	if d := r.Details.node(); d != nil {
		details := mapping(header, "details")
		if details == nil {
			details = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		for i := 0; i+1 < len(d.Content); i += 2 {
			setMapping(details, d.Content[i].Value, d.Content[i+1])
		}
		setMapping(changes, "details", details)
	}
	if len(r.DataSources) > 0 {
		sources := mapping(header, "data_sources")
		if sources == nil {
			sources = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		}
		renumbered := map[string][]int{}
		for field, list := range r.DataSources {
			for _, n := range list {
				if m := number[n]; m != 0 {
					renumbered[field] = append(renumbered[field], m)
				}
			}
		}
		if node := dataSourcesNode(renumbered); node != nil {
			for i := 0; i+1 < len(node.Content); i += 2 {
				setMapping(sources, node.Content[i].Value, node.Content[i+1])
			}
			setMapping(changes, "data_sources", sources)
		}
	}
	for i := 0; i+1 < len(changes.Content); i += 2 {
		key, value := changes.Content[i].Value, changes.Content[i+1]
		if old := mapping(header, key); old != nil {
			if old.Kind == yaml.ScalarNode && value.Kind == yaml.ScalarNode && old.Value == value.Value {
				continue
			}
		}
		var err error
		front, err = setNode(front, key, value)
		if err != nil {
			return front, err
		}
	}
	return front, nil
}
