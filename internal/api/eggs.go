package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/cpa"
)

// 鸡蛋: a short-lived endpoint shared on a forum, worth trying for a while.
//
// Adding one writes a provider straight into CPA rather than going through the
// draft — the whole point is to paste a link and have it usable immediately.
// A snapshot is taken first, as with every other write.

type eggRequest struct {
	URL       string `json:"url"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	SourceURL string `json:"source_url"`
	// Priority defaults to well above the stable sites so a fresh egg is tried
	// first while it lasts.
	Priority int `json:"priority"`
}

type eggResponse struct {
	OK      bool         `json:"ok"`
	Site    string       `json:"site"`
	Models  []string     `json:"models"`
	KeyUsed string       `json:"key_used"`
	Decoded bool         `json:"decoded"`
	View    catalog.View `json:"view"`
}

func (s *Server) handleEggs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.addEgg(w, r)
	case http.MethodDelete:
		s.removeEgg(w, r)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) addEgg(w http.ResponseWriter, r *http.Request) {
	var req eggRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	base, err := normalizeEggURL(req.URL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	key, decoded := DecodeKey(req.Key)
	if key == "" {
		writeErr(w, http.StatusBadRequest, "缺少 key")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Probe before writing anything: a dead egg should fail here, not become a
	// provider entry that only errors later.
	models, err := s.CPA.DiscoverModels(cpa.DiscoverTarget{BaseURL: base, APIKey: key})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "拉取模型列表失败："+err.Error())
		return
	}
	if len(models) == 0 {
		writeErr(w, http.StatusBadGateway, "该地址没有返回任何模型")
		return
	}

	st, err := s.load()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = eggName(st.View.Sites, base)
	}
	for _, site := range st.View.Sites {
		if site.Name == name {
			writeErr(w, http.StatusConflict, "站点名已存在："+name)
			return
		}
	}

	priority := req.Priority
	if priority == 0 {
		priority = eggPriority(st.View.Sites)
	}

	if _, err := s.Store.AddSnapshot(st.View.Fingerprint, "pre-egg", snapshotPayload(st.Snapshot)); err != nil {
		writeErr(w, http.StatusInternalServerError, "写入快照失败: "+err.Error())
		return
	}
	_ = s.Store.PruneSnapshots(s.Retain)

	provider := cpa.Provider{
		Name:          name,
		BaseURL:       base,
		Priority:      priority,
		APIKeyEntries: []cpa.APIKeyEntry{{APIKey: key}},
		Raw: map[string]any{
			"name":            name,
			"base-url":        base,
			"priority":        priority,
			"api-key-entries": []any{map[string]any{"api-key": key}},
			"models":          []any{},
		},
	}
	for _, model := range models {
		provider.Models = append(provider.Models, cpa.Model{Name: model})
	}

	next := append(append([]cpa.Provider(nil), st.Snapshot.Providers(cpa.ChannelOpenAI)...), provider)
	if err := s.CPA.PutChannel(cpa.ChannelOpenAI, next); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if err := s.Store.AddTempSite(name, strings.TrimSpace(req.SourceURL)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.RecordProbe(name, nil)

	fresh, err := s.load()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, eggResponse{
		OK:      true,
		Site:    name,
		Models:  models,
		KeyUsed: mask(key),
		Decoded: decoded,
		View:    fresh.View,
	})
}

// removeEgg is the same operation as deleting any other site.
func (s *Server) removeEgg(w http.ResponseWriter, r *http.Request) {
	site := strings.TrimSpace(r.URL.Query().Get("site"))
	if site == "" {
		writeErr(w, http.StatusBadRequest, "缺少 site")
		return
	}
	s.deleteSite(w, site)
}

// DecodeKey unwraps the base64 that shared keys usually arrive in, and leaves
// anything else alone. Reports whether it actually decoded so the UI can show
// what will be stored.
func DecodeKey(raw string) (string, bool) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", false
	}
	for _, decoder := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := decoder.DecodeString(key)
		if err != nil || len(decoded) < 8 {
			continue
		}
		text := strings.TrimSpace(string(decoded))
		if looksLikeKey(text) && text != key {
			return text, true
		}
	}
	return key, false
}

// looksLikeKey keeps base64 that happens to decode into binary from being
// mistaken for a credential.
func looksLikeKey(s string) bool {
	if len(s) < 8 || len(s) > 400 {
		return false
	}
	for _, r := range s {
		if r > unicode.MaxASCII || (!unicode.IsPrint(r)) {
			return false
		}
	}
	return strings.ContainsAny(s, "-_") || strings.HasPrefix(s, "sk") || strings.Count(s, " ") == 0
}

// normalizeEggURL accepts what people paste: with or without /v1, with or
// without a scheme, sometimes with a trailing slash.
func normalizeEggURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("缺少 URL")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("URL 无法解析：%s", raw)
	}
	parsed.RawQuery, parsed.Fragment = "", ""
	path := strings.TrimRight(parsed.Path, "/")
	path = strings.TrimSuffix(path, "/chat/completions")
	path = strings.TrimSuffix(path, "/models")
	if !strings.HasSuffix(strings.ToLower(path), "/v1") {
		path += "/v1"
	}
	parsed.Path = path
	return parsed.String(), nil
}

// eggName picks the next free 鸡蛋N.
func eggName(sites []catalog.SiteView, base string) string {
	taken := map[string]bool{}
	for _, site := range sites {
		taken[site.Name] = true
	}
	for i := 1; i < 500; i++ {
		candidate := fmt.Sprintf("鸡蛋%d", i)
		if !taken[candidate] {
			return candidate
		}
	}
	return "鸡蛋 " + base
}

// eggPriority puts a fresh egg above every stable site, and level with the
// other eggs.
func eggPriority(sites []catalog.SiteView) int {
	highest := 0
	for _, site := range sites {
		if site.Temp {
			for _, p := range site.Priorities {
				return p
			}
		}
		for _, p := range site.Priorities {
			if p > highest {
				highest = p
			}
		}
	}
	return highest + 50
}

func mask(key string) string {
	if len(key) <= 10 {
		return "***"
	}
	return key[:3] + "***" + key[len(key)-4:]
}
