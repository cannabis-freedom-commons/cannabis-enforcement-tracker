package safeurl

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// a running capture proxy whose "public" hosts all lead to target (a local test server)
func proxyFixture(t *testing.T, target string) (*browserProxyHandler, string) {
	t.Helper()
	p := &browserProxyHandler{ctx: context.Background(), auth: "Basic " + base64.StdEncoding.EncodeToString([]byte("capture:fixture")), dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, _ := net.SplitHostPort(addr)
		if host != "news.example" {
			return dialPublic(ctx, network, addr) // the real checks for everything else
		}
		return net.Dial("tcp", target)
	}}
	server := httptest.NewServer(p)
	t.Cleanup(server.Close)
	return p, server.Listener.Addr().String()
}

func TestBrowserProxyBlocksPrivateAndNonstandardDestinations(t *testing.T) {
	_, addr := proxyFixture(t, "127.0.0.1:1")
	connect := func(target string, auth bool) string {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		req := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n"
		if auth {
			req += "Proxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("capture:fixture")) + "\r\n"
		}
		conn.Write([]byte(req + "\r\n"))
		line, _ := bufio.NewReader(conn).ReadString('\n')
		return strings.TrimSpace(line)
	}
	if got := connect("news.example:443", false); !strings.Contains(got, "407") {
		t.Fatal("unauthenticated tunnel", got)
	}
	for _, target := range []string{"127.0.0.1:443", "169.254.169.254:443", "10.0.0.1:443", "[::1]:443", "localhost:443", "news.example:8443", "news.example:22"} {
		if got := connect(target, true); !strings.Contains(got, "403") {
			t.Fatal(target, got)
		}
	}
	proxyURL, _ := url.Parse("http://capture:fixture@" + addr)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	for _, target := range []string{"http://127.0.0.1/", "http://10.0.0.1/", "http://news.example:8080/"} {
		res, err := client.Get(target)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatal(target, res.StatusCode)
		}
	}
}

func TestBrowserProxyTunnelsHTTPSAndForwardsHTTP(t *testing.T) {
	// HTTPS: the proxy only joins the two ends; whatever the browser sends arrives as it is
	echo, _ := net.Listen("tcp", "127.0.0.1:0")
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	_, addr := proxyFixture(t, echo.Addr().String())
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte("CONNECT news.example:443 HTTP/1.1\r\nHost: news.example:443\r\nProxy-Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("capture:fixture")) + "\r\n\r\n"))
	reader := bufio.NewReader(conn)
	if line, _ := reader.ReadString('\n'); !strings.Contains(line, "200") {
		t.Fatal(line)
	}
	reader.ReadString('\n')
	conn.Write([]byte("client hello"))
	got := make([]byte, 12)
	if _, err = io.ReadFull(reader, got); err != nil || string(got) != "client hello" {
		t.Fatal("tunnel did not carry the bytes", string(got), err)
	}
	// HTTP: forwarded with the browser's headers, without the proxy's credentials
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("User-Agent") != "Chrome-fixture" {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte("public content"))
	}))
	defer site.Close()
	_, addr = proxyFixture(t, site.Listener.Addr().String())
	proxyURL, _ := url.Parse("http://capture:fixture@" + addr)
	req, _ := http.NewRequest("GET", "http://news.example/story", nil)
	req.Header.Set("User-Agent", "Chrome-fixture")
	res, err := (&http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || string(body) != "public content" {
		t.Fatal(res.StatusCode, string(body))
	}
}

func TestRenderedChecksUseVisibleTextAndDoNotExposeRawStructuredData(t *testing.T) {
	e := Evidence{URL: "https://news.example/a", FinalURL: "https://news.example/a", Rendered: true, Text: strings.Repeat("The public article discusses a cannabis seizure and police arrests. ", 5), Content: "<title>Public source</title><script>Hidden unsupported allegation.</script>", Screenshot: "page.png", Metadata: &Metadata{Title: "Public source", Description: "SEO description", Authors: []string{"Reporter"}, JSONLD: []json.RawMessage{json.RawMessage(`{"articleBody":"private structured-data body"}`)}}}
	e.Screenshot = "page.webp"
	check, err := Check(e)
	if err != nil || !check.Rendered || !check.Screenshot || check.Metadata == nil || check.Metadata.Description != "SEO description" {
		t.Fatal(check, err)
	}
	if !e.Supports("a cannabis seizure and police arrests") || e.Supports("Hidden unsupported allegation.") {
		t.Fatal("verification did not use visible text")
	}
	public, _ := json.Marshal(check)
	if strings.Contains(string(public), "private structured-data body") {
		t.Fatal("raw structured data leaked")
	}
	e.Text = strings.Repeat("The visible article is about an unrelated public event. ", 6)
	e.Metadata.Description = "Cannabis only appears in SEO metadata."
	check, err = Check(e)
	if err != nil || check.Mention != "" {
		t.Fatal("SEO descriptions became article evidence", check, err)
	}
}
