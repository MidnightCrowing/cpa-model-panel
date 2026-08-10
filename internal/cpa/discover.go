package cpa

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// DiscoverTarget describes how to reach a site's upstream model list.
type DiscoverTarget struct {
	BaseURL string
	Headers map[string]string
	APIKey  string
}

// DiscoverModels asks CPA to call the site's own model-list endpoint and
// returns the upstream model names. CPA performs the request so the site's
// proxy settings and network path are the ones actually used at runtime.
func (c *Client) DiscoverModels(t DiscoverTarget) ([]string, error) {
	endpoints, err := modelListEndpoints(t.BaseURL)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string, len(t.Headers)+1)
	for k, v := range t.Headers {
		headers[k] = v
	}
	if !hasHeader(headers, "authorization") && strings.TrimSpace(t.APIKey) != "" {
		headers["Authorization"] = "Bearer " + strings.TrimSpace(t.APIKey)
	}

	var lastErr error
	for _, endpoint := range endpoints {
		body, status, err := c.managementAPICall(endpoint, headers)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			lastErr = fmt.Errorf("HTTP %d from %s: %s", status, endpoint, truncate(body, 200))
			continue
		}
		names, err := parseModelList(body)
		if err != nil {
			lastErr = fmt.Errorf("bad model list from %s: %w", endpoint, err)
			continue
		}
		return names, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable model list endpoint for %q", t.BaseURL)
	}
	return nil, lastErr
}

type managementAPICallResponse struct {
	StatusCode int             `json:"status_code"`
	Body       json.RawMessage `json:"body"`
}

func (c *Client) managementAPICall(target string, headers map[string]string) ([]byte, int, error) {
	payload, err := json.Marshal(map[string]any{
		"method": http.MethodGet,
		"url":    target,
		"header": headers,
	})
	if err != nil {
		return nil, 0, err
	}
	body, status, err := c.post("/v0/management/api-call", payload)
	if err != nil {
		return nil, 0, err
	}
	if status < 200 || status >= 300 {
		return body, status, nil
	}
	var wrapped managementAPICallResponse
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, 0, fmt.Errorf("decode CPA api-call response: %w", err)
	}
	return unwrapAPICallBody(wrapped.Body), wrapped.StatusCode, nil
}

// unwrapAPICallBody handles CPA returning the upstream body either inline or
// as a JSON-encoded string.
func unwrapAPICallBody(raw json.RawMessage) []byte {
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) >= 2 && trimmed[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			return []byte(text)
		}
	}
	return raw
}

func parseModelList(raw []byte) ([]string, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if object, ok := payload.(map[string]any); ok {
		if data, exists := object["data"]; exists {
			payload = data
		} else if models, exists := object["models"]; exists {
			payload = models
		}
	}
	items, ok := payload.([]any)
	if !ok {
		return nil, fmt.Errorf("model list payload is not an array")
	}

	seen := make(map[string]struct{}, len(items))
	names := make([]string, 0, len(items))
	for _, item := range items {
		name := ""
		switch value := item.(type) {
		case string:
			name = strings.TrimSpace(value)
		case map[string]any:
			for _, key := range []string{"id", "name", "model"} {
				if s, ok := value[key].(string); ok && strings.TrimSpace(s) != "" {
					name = strings.TrimSpace(s)
					break
				}
			}
		}
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

func modelListEndpoints(base string) ([]string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid base URL %q", base)
	}
	path := strings.TrimRight(u.Path, "/")
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, "/models") {
		u.Path = path
		return []string{u.String()}, nil
	}
	if strings.HasSuffix(lower, "/v1") || strings.HasSuffix(lower, "/v1beta") {
		u.Path = path + "/models"
		return []string{u.String()}, nil
	}
	first := *u
	first.Path = path + "/v1/models"
	second := *u
	second.Path = path + "/models"
	return []string{first.String(), second.String()}, nil
}

func hasHeader(headers map[string]string, name string) bool {
	for key := range headers {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}
