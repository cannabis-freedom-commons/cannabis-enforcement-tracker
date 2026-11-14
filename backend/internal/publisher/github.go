// Package publisher owns GitHub credentials. Research never receives them.
package publisher

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/config"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/proc"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/safeurl"
	"github.com/burningtree/cannabis-freedom-index-tracker/backend/internal/store"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type APIError struct{ Status int }

func (e APIError) Error() string { return fmt.Sprintf("GitHub response %d", e.Status) }

type Client struct {
	LiveHTTP *http.Client
	Config   config.Config
	Store    *store.Store
	HTTP     *http.Client
	mu       sync.Mutex
	token    string
	expires  time.Time
}

func (c *Client) request(ctx context.Context, method, path string, in, out any, auth string) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+auth)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("GitHub request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return APIError{res.StatusCode}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4*1024*1024)).Decode(out)
}
func (c *Client) auth(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Until(c.expires) > time.Minute {
		return c.token, nil
	}
	b, err := os.ReadFile(c.Config.GitHubKeyFile)
	if err != nil {
		return "", errors.New("GitHub App key unavailable")
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return "", errors.New("invalid App PEM")
	}
	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, _ = parsed.(*rsa.PrivateKey)
	} else {
		key, _ = x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	if key == nil {
		return "", errors.New("App key must be RSA")
	}
	enc := base64.RawURLEncoding.EncodeToString
	now := time.Now()
	claims := store.JSON(map[string]any{"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(8 * time.Minute).Unix(), "iss": c.Config.GitHubAppID})
	msg := enc([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc([]byte(claims))
	hash := sha256.Sum256([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	jwt := msg + "." + enc(sig)
	var result struct {
		Token   string    `json:"token"`
		Expires time.Time `json:"expires_at"`
	}
	if err = c.request(ctx, "POST", "/app/installations/"+url.PathEscape(c.Config.GitHubInstallationID)+"/access_tokens", map[string]any{"repositories": []string{strings.Split(c.Config.GitHubRepo, "/")[1]}, "permissions": map[string]string{"contents": "write", "pull_requests": "write", "deployments": "read", "checks": "read", "statuses": "read"}}, &result, jwt); err != nil {
		return "", err
	}
	if result.Token == "" {
		return "", errors.New("missing installation token")
	}
	c.token = result.Token
	c.expires = result.Expires
	return c.token, nil
}
func (c *Client) api(ctx context.Context, method, path string, in, out any) error {
	token, err := c.auth(ctx)
	if err != nil {
		return err
	}
	return c.request(ctx, method, "/repos/"+c.Config.GitHubRepo+path, in, out, token)
}

type pull struct {
	Number   int    `json:"number"`
	URL      string `json:"html_url"`
	Merged   bool   `json:"merged"`
	MergeSHA string `json:"merge_commit_sha"`
	State    string `json:"state"`
	Draft    bool   `json:"draft"`
	Head     struct {
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func (c *Client) Propose(ctx context.Context, cid, dir, rel, previousRel, slug, title string) (string, error) {
	if !strings.HasPrefix(rel, "reports/") || !strings.HasSuffix(rel, "/"+slug+".md") || strings.Contains(rel, "..") {
		return "", errors.New("publisher path rejected")
	}
	if previousRel != "" && (!strings.HasPrefix(previousRel, "reports/") || !strings.HasSuffix(previousRel, "/"+slug+".md") || strings.Contains(previousRel, "..")) {
		return "", errors.New("publisher previous path rejected")
	}
	content, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil || len(content) > 128*1024 {
		return "", errors.New("invalid draft file")
	}
	branch := "codex/submission-" + cid
	// Persist deterministic branch before network mutation; retries search all PR states.
	_, err = c.Store.DB.ExecContext(ctx, "INSERT INTO publication(case_id,branch,slug,created_at) VALUES(?,?,?,?) ON CONFLICT DO NOTHING", cid, branch, slug, time.Now().Unix())
	if err != nil {
		return "", err
	}
	var existing []pull
	owner := strings.Split(c.Config.GitHubRepo, "/")[0]
	if err = c.api(ctx, "GET", "/pulls?state=all&head="+url.QueryEscape(owner+":"+branch), nil, &existing); err != nil {
		return "", err
	}
	if len(existing) > 0 {
		return c.remember(ctx, cid, existing[0])
	}
	// A persisted commit SHA makes retries independent of commit timestamps and PR outages.
	var created struct {
		SHA string `json:"sha"`
	}
	if err = c.Store.DB.QueryRowContext(ctx, "SELECT head_sha FROM publication WHERE case_id=?", cid).Scan(&created.SHA); err != nil {
		return "", err
	}
	if created.SHA == "" {
		cmd := proc.Own(exec.CommandContext(ctx, "git", "rev-parse", "HEAD"))
		cmd.Dir = dir
		head, err := cmd.Output()
		if err != nil {
			return "", errors.New("checkout HEAD unavailable")
		}
		sha := strings.TrimSpace(string(head))
		var commit struct {
			Tree struct {
				SHA string `json:"sha"`
			} `json:"tree"`
		}
		if err = c.api(ctx, "GET", "/git/commits/"+sha, nil, &commit); err != nil {
			return "", err
		}
		var tree struct {
			SHA string `json:"sha"`
		}
		entries := []map[string]any{{"path": rel, "mode": "100644", "type": "blob", "content": string(content)}}
		if previousRel != "" && previousRel != rel {
			entries = append(entries, map[string]any{"path": previousRel, "mode": "100644", "type": "blob", "sha": nil})
		}
		if err = c.api(ctx, "POST", "/git/trees", map[string]any{"base_tree": commit.Tree.SHA, "tree": entries}, &tree); err != nil {
			return "", err
		}
		if err = c.api(ctx, "POST", "/git/commits", map[string]any{"message": "Report draft: " + title, "tree": tree.SHA, "parents": []string{sha}}, &created); err != nil {
			return "", err
		}
		if _, err = c.Store.DB.ExecContext(ctx, "UPDATE publication SET head_sha=? WHERE case_id=?", created.SHA, cid); err != nil {
			return "", err
		}
	}
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	err = c.api(ctx, "GET", "/git/ref/heads/"+branch, nil, &ref)
	if err != nil {
		var apiErr APIError
		if !errors.As(err, &apiErr) || apiErr.Status != 404 {
			return "", err
		}
		if err = c.api(ctx, "POST", "/git/refs", map[string]string{"ref": "refs/heads/" + branch, "sha": created.SHA}, nil); err != nil {
			return "", err
		}
	} else {
		if ref.Object.SHA != created.SHA {
			return "", errors.New("branch already exists; operator reconciliation required")
		}
	}
	var pr pull
	body := "Report from a public submission. Source retrieval, quotation support, schema and build checks passed in an isolated checkout. The report remains `reported`; automated checks do not establish factual truth.\n\nCase: " + cid
	if c.Config.AutoPublish {
		body += "\n\nAutomatic publication: merge only the validated head after the Validate report check and all other checks pass. Then confirm Pages deployment and the live URL."
	} else {
		body += "\n\nHuman review is required before publication."
	}
	if err = c.api(ctx, "POST", "/pulls", map[string]any{"title": "Report: " + title, "head": branch, "base": "main", "body": body, "draft": !c.Config.AutoPublish}, &pr); err != nil {
		return "", err
	}
	return c.remember(ctx, cid, pr)
}
func (c *Client) remember(ctx context.Context, cid string, p pull) (string, error) {
	_, err := c.Store.DB.ExecContext(ctx, "UPDATE publication SET pr_number=? WHERE case_id=?", p.Number, cid)
	return p.URL, err
}

type deployment struct {
	ID          int64  `json:"id"`
	SHA         string `json:"sha"`
	Environment string `json:"environment"`
}
type deployStatus struct {
	State          string `json:"state"`
	EnvironmentURL string `json:"environment_url"`
}

// PublicationReady is deliberately conjunctive: merge, Pages success at merge SHA, live page.
func PublicationReady(merged, pages, live bool) bool { return merged && pages && live }
func (c *Client) Reconcile(ctx context.Context) error {
	rows, err := c.Store.DB.QueryContext(ctx, "SELECT p.case_id,p.pr_number,p.slug FROM publication p JOIN cases c ON c.id=p.case_id WHERE p.pr_number IS NOT NULL AND c.status IN ('publishing','awaiting_review','merged','deploying')")
	if err != nil {
		return err
	}
	type item struct {
		cid    string
		number int
		slug   string
	}
	var items []item
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.cid, &v.number, &v.slug); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range items {
		if err = c.reconcileOne(ctx, v.cid, v.number, v.slug); err != nil {
			return err
		}
	}
	return nil
}
func (c *Client) reconcileOne(ctx context.Context, cid string, number int, slug string) error {
	var p pull
	if err := c.api(ctx, "GET", fmt.Sprintf("/pulls/%d", number), nil, &p); err != nil {
		return err
	}
	if !p.Merged {
		if p.State == "closed" {
			return c.Store.Transition(ctx, cid, "failed", "The report proposal was closed without publication.", "", p.URL)
		}
		if c.Config.AutoPublish {
			merged, err := c.automaticMerge(ctx, cid, p)
			if err != nil {
				return err
			}
			if merged {
				return c.Store.Transition(ctx, cid, "merged", "The validated report was merged automatically; waiting for deployment.", slug, p.URL)
			}
		}
		return nil
	}
	var status string
	if err := c.Store.DB.QueryRowContext(ctx, "SELECT status FROM cases WHERE id=?", cid).Scan(&status); err != nil {
		return err
	}
	if status == "awaiting_review" || status == "publishing" {
		if err := c.Store.Transition(ctx, cid, "merged", "The proposal was merged; waiting for a successful site deployment.", slug, p.URL); err != nil {
			return err
		}
	}
	if err := c.Store.Transition(ctx, cid, "deploying", "Checking the Pages deployment and live report.", slug, p.URL); err != nil {
		return err
	}
	var deployments []deployment
	if err := c.api(ctx, "GET", "/deployments?sha="+url.QueryEscape(p.MergeSHA)+"&environment=github-pages&per_page=20", nil, &deployments); err != nil {
		return err
	}
	pages := false
	for _, d := range deployments {
		if d.SHA != p.MergeSHA || d.Environment != "github-pages" {
			continue
		}
		var statuses []deployStatus
		if err := c.api(ctx, "GET", fmt.Sprintf("/deployments/%d/statuses?per_page=1", d.ID), nil, &statuses); err != nil {
			return err
		}
		if len(statuses) > 0 && statuses[0].State == "success" {
			pages = true
			break
		}
	}
	if !pages {
		return nil
	}
	live := false
	req, err := http.NewRequestWithContext(ctx, "GET", c.Config.PublicURL+"/report/"+slug+"/", nil)
	if err != nil {
		return err
	}
	client := c.LiveHTTP
	if client == nil {
		client = safeurl.Client()
	}
	res, err := client.Do(req)
	if err == nil {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 1024*1024))
		expected := c.Config.PublicURL + "/report/" + slug + "/"
		live = res.StatusCode == 200 && res.Request != nil && res.Request.URL.String() == expected && strings.Contains(string(b), `rel="canonical" href="`+expected+`"`)
	}
	if PublicationReady(p.Merged, pages, live) {
		return c.Store.Transition(ctx, cid, "published", "The report was merged, deployed successfully and confirmed on the live site.", slug, p.URL)
	}
	return nil
}
