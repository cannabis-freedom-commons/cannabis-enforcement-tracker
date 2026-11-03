package research

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CheckReport checks a report file the way the site will read it, without building the site: the
// front matter against the rules of the content schema (src/content.config.ts) and the text against
// the report tests (scripts/reports.test.mjs): its place in reports/<year>/<month>/, its slug, every
// citation naming a source and every source cited. dir is the repository checkout (for the country
// list); rel the file's path in it. The errors name the field, so a failure says what to fix.
func CheckReport(dir, rel, content string) error {
	m := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n(.*)$`).FindStringSubmatch(content)
	if m == nil {
		return errors.New("no front matter")
	}
	var data map[string]any
	if err := yaml.Unmarshal([]byte(m[1]), &data); err != nil {
		return fmt.Errorf("front matter is not valid YAML: %v", err)
	}
	if data == nil {
		return errors.New("empty front matter")
	}
	delete(data, "updated_by_models") // retired; the site drops it too
	body := m[2]
	countries, err := countryIDs(dir)
	if err != nil {
		return err
	}
	c := checker{}
	c.only(data, "", "code", "date", "country", "place", "coordinates", "precision", "type", "also", "title", "summary", "figures", "cover", "media", "status", "cfi_id", "tags", "created_by_model", "submission", "open_questions", "sources")
	c.match("code", data["code"], `^[0-9A-HJKMNP-TV-Z]{6}$`)
	date := c.date("date", data["date"], true)
	if id, _ := data["country"].(string); !countries[id] {
		c.fail("country: %q is not a country id of the Cannabis Freedom Index", data["country"])
	}
	c.text("place", data["place"], 0, false)
	if v, ok := data["coordinates"]; ok {
		pair, ok := v.([]any)
		lat, okLat := number(pair, 0)
		lon, okLon := number(pair, 1)
		if !ok || len(pair) != 2 || !okLat || !okLon || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			c.fail("coordinates: must be [lat, lon] in range")
		}
		if _, ok := data["precision"]; !ok {
			c.fail("coordinates need a precision")
		}
	}
	if v, ok := data["precision"]; ok {
		c.oneOf("precision", v, "exact", "locality", "district", "region", "country")
	}
	reportTypes := []string{"eradication", "seizure", "raid", "arrest", "prosecution", "sentence", "execution", "death"}
	c.oneOf("type", data["type"], reportTypes...)
	if v, ok := data["also"]; ok {
		for i, t := range c.list("also", v) {
			c.oneOf(fmt.Sprintf("also[%d]", i), t, reportTypes...)
			if t == data["type"] {
				c.fail("also repeats the main type")
			}
		}
	}
	c.text("title", data["title"], 120, true)
	c.text("summary", data["summary"], 400, true)
	if v, ok := data["figures"]; ok {
		f := c.object("figures", v)
		c.only(f, "figures.", "arrested", "charged", "kg_seized", "plants_destroyed", "hectares", "prison", "fine", "value", "deaths")
		for _, k := range []string{"arrested", "charged", "plants_destroyed", "deaths"} {
			if n, ok := f[k]; ok {
				if x, isNum := toNumber(n); !isNum || x < 0 || x != float64(int64(x)) {
					c.fail("figures.%s: a whole number, 0 or more", k)
				}
			}
		}
		for _, k := range []string{"kg_seized", "hectares"} {
			if n, ok := f[k]; ok {
				if x, isNum := toNumber(n); !isNum || x < 0 {
					c.fail("figures.%s: a number, 0 or more", k)
				}
			}
		}
		c.text("figures.prison", f["prison"], 0, false)
		for _, k := range []string{"fine", "value"} {
			if v, ok := f[k]; ok {
				money := c.object("figures."+k, v)
				c.only(money, "figures."+k+".", "amount", "currency")
				if x, isNum := toNumber(money["amount"]); !isNum || x <= 0 {
					c.fail("figures.%s.amount: a positive number", k)
				}
				c.match("figures."+k+".currency", money["currency"], `^[A-Z]{3}$`)
			}
		}
	}
	if v, ok := data["cover"]; ok {
		cover := c.object("cover", v)
		c.only(cover, "cover.", "image", "alt", "credit", "licence", "url", "media_url")
		for _, k := range []string{"image", "alt", "credit", "licence"} {
			c.text("cover."+k, cover[k], 0, true)
		}
		c.url("cover.url", cover["url"], true)
		c.url("cover.media_url", cover["media_url"], false)
	}
	if v, ok := data["media"]; ok {
		for i, raw := range c.list("media", v) {
			name := fmt.Sprintf("media[%d]", i)
			item := c.object(name, raw)
			c.only(item, name+".", "kind", "url", "media_url", "publisher", "caption", "archived")
			c.oneOf(name+".kind", item["kind"], "photo", "video")
			c.url(name+".url", item["url"], true)
			c.url(name+".media_url", item["media_url"], false)
			c.url(name+".archived", item["archived"], false)
			c.text(name+".publisher", item["publisher"], 0, true)
			c.text(name+".caption", item["caption"], 0, true)
		}
	}
	c.oneOf("status", data["status"], "verified", "reported", "disputed")
	if v, ok := data["cfi_id"]; ok {
		c.match("cfi_id", v, `^[a-z0-9-]+$`)
	}
	if v, ok := data["tags"]; ok {
		for i, t := range c.list("tags", v) {
			c.match(fmt.Sprintf("tags[%d]", i), t, `^[a-z0-9-]+$`)
		}
	}
	if s, ok := data["created_by_model"].(string); !ok || strings.TrimSpace(s) == "" || len(s) > 300 {
		c.fail("created_by_model is required (at most 300 characters)")
	}
	if v, ok := data["submission"]; ok {
		c.match("submission", v, `^[0-9a-f]{32}$`)
	}
	if v, ok := data["open_questions"]; ok {
		list := c.list("open_questions", v)
		if len(list) > 30 {
			c.fail("open_questions: at most 30")
		}
		for i, q := range list {
			c.text(fmt.Sprintf("open_questions[%d]", i), q, 4000, true)
		}
	}
	sources := c.list("sources", data["sources"])
	if len(sources) == 0 {
		c.fail("a report needs at least one source")
	}
	for i, raw := range sources {
		name := fmt.Sprintf("sources[%d]", i)
		s := c.object(name, raw)
		c.only(s, name+".", "title", "url", "publisher", "type", "authors", "container", "year", "isbn", "locator", "date", "archived", "language")
		c.text(name+".title", s["title"], 0, true)
		c.text(name+".publisher", s["publisher"], 0, true)
		c.url(name+".url", s["url"], false)
		c.url(name+".archived", s["archived"], false)
		if _, ok := s["type"]; ok {
			c.oneOf(name+".type", s["type"], "book", "chapter", "journal", "report", "law", "other")
		} else if _, ok := s["url"]; !ok {
			c.fail("%s needs a url, or a type (book, journal, report, law) with its details", name)
		}
		if _, ok := s["date"]; ok {
			c.date(name+".date", s["date"], false)
		}
		if _, ok := s["language"]; ok {
			c.match(name+".language", s["language"], `^[a-z]{2}$`)
		}
	}
	// the text: where the file sits, its slug, its citations
	parts := strings.Split(filepath.ToSlash(rel), "/")
	slug := strings.TrimSuffix(parts[len(parts)-1], ".md")
	if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(slug) {
		c.fail("the file name is the slug: lower case, digits and hyphens")
	}
	// reports/<year>/<month>/[…/]<slug>.md, as the report tests allow
	if len(parts) < 4 || parts[0] != "reports" || (!date.IsZero() && parts[1]+"-"+parts[2] != date.Format("2006-01")) {
		c.fail("the file must sit in reports/<year>/<month>/ of its date")
	}
	cited := map[int]bool{}
	for _, m := range regexp.MustCompile(`\[@([^\]]+)\]`).FindAllStringSubmatch(body, -1) {
		for _, part := range strings.Split(m[1], ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 1 || n > len(sources) {
				c.fail("[@%s] names no source", strings.TrimSpace(part))
				continue
			}
			cited[n] = true
		}
	}
	for i := 1; i <= len(sources); i++ {
		if !cited[i] {
			c.fail("source %d is never cited ([@%d])", i, i)
		}
	}
	minimum := 200
	if _, ok := data["cfi_id"]; ok {
		minimum = 60
	}
	if len(strings.TrimSpace(body)) <= minimum {
		c.fail("the text is too short")
	}
	if len(c.problems) > 0 {
		if len(c.problems) > 8 {
			c.problems = append(c.problems[:8], "…")
		}
		return errors.New(strings.Join(c.problems, "; "))
	}
	return nil
}

func countryIDs(dir string) (map[string]bool, error) {
	b, err := os.ReadFile(filepath.Join(dir, "src/data/countries.json"))
	if err != nil {
		return nil, fmt.Errorf("country list unavailable: %v", err)
	}
	var list []struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("country list unreadable: %v", err)
	}
	ids := map[string]bool{}
	for _, c := range list {
		ids[c.ID] = true
	}
	return ids, nil
}

type checker struct{ problems []string }

func (c *checker) fail(format string, args ...any) {
	c.problems = append(c.problems, fmt.Sprintf(format, args...))
}

// only: no field the schema does not know (it is strict)
func (c *checker) only(m map[string]any, prefix string, keys ...string) {
	known := map[string]bool{}
	for _, k := range keys {
		known[k] = true
	}
	for k := range m {
		if !known[k] {
			c.fail("%s%s: unknown field", prefix, k)
		}
	}
}

func (c *checker) object(name string, v any) map[string]any {
	m, ok := v.(map[string]any)
	if !ok {
		c.fail("%s: must be an object", name)
		return map[string]any{}
	}
	return m
}

func (c *checker) list(name string, v any) []any {
	l, ok := v.([]any)
	if !ok {
		c.fail("%s: must be a list", name)
	}
	return l
}

// text: a non-empty string of at most max characters (0: no limit); required or optional
func (c *checker) text(name string, v any, max int, required bool) {
	if v == nil {
		if required {
			c.fail("%s is required", name)
		}
		return
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		c.fail("%s: must be text", name)
	} else if max > 0 && len([]rune(s)) > max {
		c.fail("%s: at most %d characters", name, max)
	}
}

func (c *checker) match(name string, v any, pattern string) {
	s, ok := v.(string)
	if !ok || !regexp.MustCompile(pattern).MatchString(s) {
		c.fail("%s: %v does not match %s", name, v, pattern)
	}
}

func (c *checker) oneOf(name string, v any, allowed ...string) {
	s, _ := v.(string)
	for _, a := range allowed {
		if s == a {
			return
		}
	}
	c.fail("%s: %v is not one of %s", name, v, strings.Join(allowed, ", "))
}

func (c *checker) url(name string, v any, required bool) {
	if v == nil {
		if required {
			c.fail("%s is required", name)
		}
		return
	}
	s, _ := v.(string)
	if u, err := url.Parse(s); err != nil || u.Scheme == "" || u.Host == "" {
		c.fail("%s: not a valid URL", name)
	}
}

// date: YYYY-MM-DD (YAML may already have read it as a time)
func (c *checker) date(name string, v any, required bool) time.Time {
	switch d := v.(type) {
	case time.Time:
		return d
	case string:
		if t, err := time.Parse("2006-01-02", d); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, d); err == nil {
			return t
		}
	case nil:
		if !required {
			return time.Time{}
		}
	}
	c.fail("%s: not a date (YYYY-MM-DD)", name)
	return time.Time{}
}

func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

func number(list []any, i int) (float64, bool) {
	if i >= len(list) {
		return 0, false
	}
	return toNumber(list[i])
}
