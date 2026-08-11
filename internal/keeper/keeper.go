// Package keeper reads request statistics from CPA Usage Keeper.
//
// Keeper records one event per upstream request with the model, the source
// (site plus masked key), whether it failed and how long it took. Aggregating
// those by (site, model) is what lets the matrix page colour a cell by whether
// that model actually works at that site.
package keeper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"
)

type Client struct {
	BaseURL  string
	Password string
	HTTP     *http.Client

	mu       sync.Mutex
	loggedIn bool
}

func NewClient(baseURL, password string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Password: password,
		HTTP:     &http.Client{Timeout: 30 * time.Second, Jar: jar},
	}
}

// Configured reports whether the panel was given somewhere to look.
func (c *Client) Configured() bool {
	return c != nil && c.BaseURL != "" && c.Password != ""
}

// Keeper refuses any request that does not look like it came from its own
// frontend, and serves its SPA for unknown paths, so both the prefix and this
// header are load-bearing.
const (
	apiPrefix     = "/api/v1"
	fetchHeader   = "X-CPA-Usage-Keeper-Request"
	fetchHeaderOn = "fetch"
)

func (c *Client) do(method, path string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.BaseURL+apiPrefix+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set(fetchHeader, fetchHeaderOn)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.HTTP.Do(req)
}

func (c *Client) login() error {
	payload, err := json.Marshal(map[string]string{"password": c.Password})
	if err != nil {
		return err
	}
	res, err := c.do(http.MethodPost, "/auth/login", payload)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 500))
		return fmt.Errorf("keeper 登录失败: %s: %s", res.Status, body)
	}
	return nil
}

// get issues a request, logging in once if the session is missing or expired.
func (c *Client) get(path string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for attempt := 0; attempt < 2; attempt++ {
		if !c.loggedIn {
			if err := c.login(); err != nil {
				return nil, err
			}
			c.loggedIn = true
		}
		res, err := c.do(http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 32<<20))
		res.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if res.StatusCode == http.StatusUnauthorized {
			c.loggedIn = false
			continue
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, fmt.Errorf("keeper %s: %s: %s", path, res.Status, truncate(body, 200))
		}
		return body, nil
	}
	return nil, fmt.Errorf("keeper 认证反复失效")
}

type event struct {
	Timestamp string `json:"timestamp"`
	Model     string `json:"model"`
	Alias     string `json:"model_alias"`
	Source    string `json:"source"`
	Type      string `json:"source_type"`
	Failed    bool   `json:"failed"`
	LatencyMs int    `json:"latency_ms"`
}

type eventsPage struct {
	Events     []event `json:"events"`
	TotalCount int     `json:"total_count"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	TotalPages int     `json:"total_pages"`
}

// Cell is the outcome of one (site, model) pair over the requested window.
type Cell struct {
	Site      string `json:"site"`
	Model     string `json:"model"`
	OK        int    `json:"ok"`
	Failed    int    `json:"failed"`
	LatencyMs int    `json:"latency_ms"`
	LastAt    string `json:"last_at"`
}

type Stats struct {
	Range     string `json:"range"`
	UpdatedAt string `json:"updated_at"`
	Events    int    `json:"events"`
	Cells     []Cell `json:"cells"`
}

// Stats pulls the window's events and folds them into per-(site, model) counts.
//
// The volume is small — a busy day is a few hundred events — so paging through
// them and aggregating here costs less than asking Keeper for a shape it does
// not offer.
func (c *Client) Stats(window string) (Stats, error) {
	if window == "" {
		window = "24h"
	}
	type key struct{ site, model string }
	totals := map[key]*Cell{}
	latency := map[key][]int{}
	count := 0

	for page := 1; page <= 20; page++ {
		raw, err := c.get(fmt.Sprintf("/usage/events?range=%s&page=%d&pageSize=500", window, page))
		if err != nil {
			return Stats{}, err
		}
		var decoded eventsPage
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return Stats{}, fmt.Errorf("解析 keeper 事件失败: %w", err)
		}
		for _, e := range decoded.Events {
			count++
			k := key{site: SiteOf(e.Source), model: e.Model}
			cell := totals[k]
			if cell == nil {
				cell = &Cell{Site: k.site, Model: k.model}
				totals[k] = cell
			}
			if e.Failed {
				cell.Failed++
			} else {
				cell.OK++
				if e.LatencyMs > 0 {
					latency[k] = append(latency[k], e.LatencyMs)
				}
			}
			if e.Timestamp > cell.LastAt {
				cell.LastAt = e.Timestamp
			}
		}
		if decoded.TotalPages <= page || len(decoded.Events) == 0 {
			break
		}
	}

	out := Stats{
		Range:     window,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Events:    count,
		Cells:     make([]Cell, 0, len(totals)),
	}
	for k, cell := range totals {
		if samples := latency[k]; len(samples) > 0 {
			sum := 0
			for _, v := range samples {
				sum += v
			}
			cell.LatencyMs = sum / len(samples)
		}
		out.Cells = append(out.Cells, *cell)
	}
	return out, nil
}

// SiteOf strips the masked key Keeper appends to every source label:
// "CHY公益站 @ 1Om*********9mdv5B" → "CHY公益站".
func SiteOf(source string) string {
	if i := strings.LastIndex(source, " @ "); i > 0 {
		return strings.TrimSpace(source[:i])
	}
	return strings.TrimSpace(source)
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
