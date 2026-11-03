package research

import (
	"errors"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The answer becomes a file laid out like the hand-written reports: readable YAML, paragraphs.
func TestMarkdownLooksLikeAReport(t *testing.T) {
	r := valid()
	arrested, kg := 2, 12.5
	r.Also = []string{"arrest"}
	r.Figures = &Figures{Arrested: &arrested, KgSeized: &kg, Value: &Money{Amount: 300000, Currency: "THB"}}
	r.Tags = []string{"opium"}
	r.Sources[0].Date = "2026-01-03"
	r.Sources[0].Language = "th"
	r.Summary = strings.Repeat("Police raided a farm and took two men away. ", 4)
	r.Facts = []Fact{
		{Text: "Police raided the farm.", Source: 1, Quote: "q", Paragraph: 1},
		{Text: "Two men were arrested.", Source: 1, Quote: "q", Paragraph: 1},
		{Text: "The farm grew opium too.", Source: 1, Quote: "q", Paragraph: 2},
	}
	r.Submission = "0123456789abcdef0123456789abcdef"
	md := r.Markdown()
	for _, want := range []string{
		"---\ncode: MZ48CH\ndate: 2026-01-02\ncountry: th\nplace: Chiang Mai\ncoordinates: [18.79, 98.98]\nprecision: locality\ntype: raid\nalso: [arrest]\n",
		"summary: >-\n  Police raided",
		"figures:\n  arrested: 2\n  kg_seized: 12.5\n  value:\n    amount: 300000\n    currency: THB\n",
		"status: reported\ncreated_by_model: fixture/research-model\nsubmission: 0123456789abcdef0123456789abcdef\ntags: [opium]\nsources:\n  - title: News story\n    url: https://news.example/a\n    publisher: Publisher\n    date: 2026-01-03\n    language: th\n---\n",
		"\n\nPolice raided the farm. [@1] Two men were arrested. [@1]\n\nThe farm grew opium too. [@1]\n",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in\n%s", want, md)
		}
	}
	// model text that looks like YAML stays a string
	r.Title = "status: verified\nx"
	if strings.Contains(r.Markdown(), "\nstatus: verified") {
		t.Fatal("title escaped into front matter")
	}
}

// Photos of the event are linked as media, only for pages the report cites.
func TestMediaLinksOnlyForCitedPages(t *testing.T) {
	r := valid()
	r.Media = []Media{{Kind: "photo", URL: r.Sources[0].URL, MediaURL: "https://images.example/plants.jpg?size=original&token=abc", Publisher: "Publisher", Caption: "Police photos of the plants. Photo: Police"}}
	if err := r.Validate(map[string]bool{"th": true}); err != nil {
		t.Fatal(err)
	}
	var header struct {
		Media []struct {
			URL      string `yaml:"url"`
			MediaURL string `yaml:"media_url"`
		} `yaml:"media"`
	}
	if err := yaml.Unmarshal([]byte(strings.SplitN(r.Markdown(), "---", 3)[1]), &header); err != nil {
		t.Fatal(err)
	}
	if len(header.Media) != 1 || header.Media[0].URL != r.Media[0].URL || header.Media[0].MediaURL != r.Media[0].MediaURL {
		t.Fatal("source page or exact asset URL lost", header)
	}
	r.Media[0].URL = "https://elsewhere.example/photo"
	if r.Validate(map[string]bool{"th": true}) == nil {
		t.Fatal("media link to an uncited page accepted")
	}
}

func TestCollectedMediaNeedsAssetAddress(t *testing.T) {
	for _, kind := range []string{"photo", "video"} {
		for _, address := range []string{"", "javascript:alert(1)", "/relative.jpg", "http://127.0.0.1/private.jpg"} {
			r := valid()
			r.Media = []Media{{Kind: kind, URL: r.Sources[0].URL, MediaURL: address, Publisher: "Publisher", Caption: "Footage of the raid"}}
			if err := r.Validate(map[string]bool{"th": true}); err == nil {
				t.Fatalf("accepted %s address %q", kind, address)
			}
		}
		r := valid()
		r.Media = []Media{{Kind: kind, URL: r.Sources[0].URL, MediaURL: "https://cdn.example/asset?signature=123", Publisher: "Publisher", Caption: "Footage of the raid"}}
		if err := r.Validate(map[string]bool{"th": true}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpdatesKeepDistinctMediaFromSamePage(t *testing.T) {
	dir := t.TempDir()
	r := valid()
	file := filepath.Join(dir, "reports/2026/01", r.Slug+".md")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(r.Markdown()), 0644); err != nil {
		t.Fatal(err)
	}
	r.Outcome = "existing_case"
	r.ExistingReport = r.Slug
	for _, asset := range []string{"https://images.example/one.jpg?size=full", "https://images.example/two.mp4?signature=abc"} {
		kind := "photo"
		if strings.Contains(asset, ".mp4") {
			kind = "video"
		}
		r.Media = append(r.Media, Media{Kind: kind, URL: r.Sources[0].URL, MediaURL: asset, Publisher: "Publisher", Caption: "The raid"})
	}
	for i := 0; i < 2; i++ {
		draft, err := PrepareDraft(dir, r)
		if errors.Is(err, ErrUnchanged) && i == 1 {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var header struct {
			Media []struct {
				MediaURL string `yaml:"media_url"`
			} `yaml:"media"`
		}
		if err = yaml.Unmarshal([]byte(strings.SplitN(draft.Content, "---", 3)[1]), &header); err != nil {
			t.Fatal(err)
		}
		if len(header.Media) != 2 || header.Media[0].MediaURL != r.Media[0].MediaURL || header.Media[1].MediaURL != r.Media[1].MediaURL {
			t.Fatal("assets lost or duplicated", header)
		}
		if err = os.WriteFile(file, []byte(draft.Content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
