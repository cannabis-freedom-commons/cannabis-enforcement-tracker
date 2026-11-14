package safeurl

import (
	"strings"
	"testing"
)

func page(title, body string) Evidence {
	return Evidence{URL: "https://www.example.com/a", FinalURL: "https://www.example.com/a", Content: "<html><head><title>" + title + "</title><script>var weed = 1;</script></head><body>" + body + "</body></html>"}
}

func TestCheckFindsTheWordForCannabis(t *testing.T) {
	c, err := Check(page("ตำรวจทำลายไร่กัญชา", "<p>"+strings.Repeat("เจ้าหน้าที่ตำรวจตระเวนชายแดนเข้าตรวจสอบพื้นที่ป่าสงวนแห่งชาติ ", 10)+"</p>"))
	if err != nil || c.Mention != "กัญชา" || c.Host != "example.com" || c.Title != "ตำรวจทำลายไร่กัญชา" {
		t.Fatalf("%+v %v", c, err)
	}
	if !strings.Contains(c.Describe(), "mentioning cannabis (กัญชา)") {
		t.Fatal(c.Describe())
	}
}

func TestCheckIgnoresScriptsAndPartWords(t *testing.T) {
	// "weed" only in a script and "hashtag" in the text are not mentions
	c, err := Check(page("Football results", "<p>"+strings.Repeat("The match ended two all. Share it with the hashtag #cup. ", 10)+"</p>"))
	if err != nil || c.Mention != "" {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestCheckRejectsAPageWithoutText(t *testing.T) {
	if _, err := Check(page("App", `<div id="root"></div>`)); err == nil || !strings.Contains(err.Error(), "almost no readable text") {
		t.Fatalf("want a no-text error, got %v", err)
	}
}

func TestTextWrapAndContains(t *testing.T) {
	page := `<html><script>var x</script><p>First   paragraph &amp; more.</p><div>Second<br>line</div></html>`
	if got := Text(page); got != "First paragraph & more.\nSecond\nline" {
		t.Fatalf("%q", got)
	}
	long := strings.Repeat("word ", 500)
	for _, line := range strings.Split(Wrap(long, 1000), "\n") {
		if len([]rune(line)) > 1000 {
			t.Fatal("line not wrapped")
		}
	}
	if !Contains(page, "paragraph & more. Second") || Contains(page, "invented quote") {
		t.Fatal("quotation check")
	}
}

func TestCFIRecognitionCoversInflectionsAccentsAndUnicodeBoundaries(t *testing.T) {
	for _, text := range []string{
		"Polícia odhalila pestovanie konopy v Ilave.",
		"Muž pestoval konopu a predával konopné produkty.",
		"Dlouhodobě opatřoval pervitin a marihuanu.",
		"Policie zajistila marihuany a konopím.",
		"Policia apreendeu maconha.",
		"Policie nasla KONOPI.",
		"警察查获大量大麻并拘留嫌疑人。",
		"เจ้าหน้าที่พบกัญชาจำนวนมาก",
		"Полиция обнаружила коноплю.",
		"La policía decomisó hachís.",
	} {
		t.Run(text, func(t *testing.T) {
			c, err := Check(page("Public source", strings.Repeat(text+" ", 20)))
			if err != nil || c.Mention == "" {
				t.Fatalf("missing CFI term: %+v %v", c, err)
			}
		})
	}
	for _, text := range []string{"A hashtag was added to the scoreboard.", "The weeds in the garden need weeding.", "A joint statement about a grass football pitch.", "The market expanded in spring.", "The report mentions a trapot and a hashcode."} {
		if got := cannabisMention(text); got != "" {
			t.Errorf("unrelated text matched %q: %s", got, text)
		}
	}
	if got := cannabisMention("konopi\u0301"); got == "" {
		t.Fatal("decomposed diacritics were missed")
	}
}

// A quotation typed with other quotation marks, dashes or an ellipsis than the page still counts;
// other words never do.
func TestContainsIgnoresTypography(t *testing.T) {
	page := "<p>Police said the man’s car held “2 kg of gan­ja” – enough to sell…</p>"
	for _, quote := range []string{
		`Police said the man's car held "2 kg of ganja" - enough to sell...`,
		"Police said the man’s car held “2 kg of ganja” — enough",
	} {
		if !Contains(page, quote) {
			t.Fatalf("not found: %s", quote)
		}
	}
	if Contains(page, `Police said the man's car held "3 kg of ganja"`) || Contains(page, "police said the man's") {
		t.Fatal("other words or case must not match")
	}
}

// A quotation the page does not contain is answered with the page's closest sentence.
func TestClosestPassage(t *testing.T) {
	page := "<p>The raid took place on Monday.</p><p>Officers seized 1 quintal 45 kg of ganja and Rs 3.6 lakh in cash from the house.</p>"
	got, score := Closest(page, "Officers seized 145 kg of ganja and Rs 3.6 lakh cash")
	if got != "Officers seized 1 quintal 45 kg of ganja and Rs 3.6 lakh in cash from the house." || score < 0.7 {
		t.Fatalf("%q %.2f", got, score)
	}
}
