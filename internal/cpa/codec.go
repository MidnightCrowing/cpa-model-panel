package cpa

import (
	"encoding/json"
	"strings"
)

// parseProvider decodes one channel entry, keeping the original object in Raw.
func parseProvider(raw json.RawMessage) (Provider, error) {
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return Provider{}, err
	}
	p := Provider{Raw: generic}
	if v, ok := generic["name"].(string); ok {
		p.Name = v
	}
	if v, ok := generic["base-url"].(string); ok {
		p.BaseURL = v
	}
	if v, ok := generic["prefix"].(string); ok {
		p.Prefix = v
	}
	if v, ok := generic["disabled"].(bool); ok {
		p.Disabled = v
	}
	if v, ok := generic["disable-cooling"].(bool); ok {
		p.DisableCooling = v
	}
	if v, ok := generic["support-prompt-cache-key"].(bool); ok {
		p.SupportPromptCacheKey = v
	}
	if v, ok := generic["priority"].(float64); ok {
		p.Priority = int(v)
	}
	if h, ok := generic["headers"].(map[string]any); ok {
		p.Headers = map[string]string{}
		for k, val := range h {
			if s, ok := val.(string); ok {
				p.Headers[k] = s
			}
		}
	}
	// codex/claude entries carry a flat api-key; openai entries use api-key-entries.
	if k, ok := generic["api-key"].(string); ok && strings.TrimSpace(k) != "" {
		proxyURL, _ := generic["proxy-url"].(string)
		p.APIKeyEntries = []APIKeyEntry{{APIKey: k, ProxyURL: proxyURL}}
	}
	if entries, ok := generic["api-key-entries"].([]any); ok {
		for _, item := range entries {
			km, ok := item.(map[string]any)
			if !ok {
				continue
			}
			e := APIKeyEntry{}
			if v, ok := km["api-key"].(string); ok {
				e.APIKey = v
			}
			if v, ok := km["proxy-url"].(string); ok {
				e.ProxyURL = v
			}
			if v, ok := km["weight"].(float64); ok {
				w := int(v)
				e.Weight = &w
			}
			p.APIKeyEntries = append(p.APIKeyEntries, e)
		}
	}
	if models, ok := generic["models"].([]any); ok {
		for _, item := range models {
			mm, ok := item.(map[string]any)
			if !ok {
				continue
			}
			p.Models = append(p.Models, ModelFromMap(mm))
		}
	}
	return p, nil
}

// ModelFromMap builds a Model from a generic JSON object, keeping Raw.
func ModelFromMap(m map[string]any) Model {
	out := Model{Raw: cloneMap(m)}
	if v, ok := m["name"].(string); ok {
		out.Name = v
	}
	if v, ok := m["alias"].(string); ok {
		out.Alias = v
	}
	return out
}

// ToMap renders a model for writing. The original object is preserved; only
// name and alias are overlaid, and an empty alias removes the key entirely so
// clearing a remap in the panel really clears it in CPA.
func (m Model) ToMap() map[string]any {
	out := cloneMap(m.Raw)
	if out == nil {
		out = map[string]any{}
	}
	out["name"] = m.Name
	if strings.TrimSpace(m.Alias) == "" {
		delete(out, "alias")
	} else {
		out["alias"] = m.Alias
	}
	return out
}

// providerToWriteMap renders a provider for writing. Everything except the
// model list and priority round-trips untouched from Raw, which is what keeps
// api keys, auth-index and unknown flags intact.
func providerToWriteMap(p Provider) map[string]any {
	out := cloneMap(p.Raw)
	if out == nil {
		out = map[string]any{}
		if p.Name != "" {
			out["name"] = p.Name
		}
		out["base-url"] = p.BaseURL
		if len(p.APIKeyEntries) > 0 {
			keys := make([]map[string]any, 0, len(p.APIKeyEntries))
			for _, e := range p.APIKeyEntries {
				km := map[string]any{"api-key": e.APIKey}
				if e.ProxyURL != "" {
					km["proxy-url"] = e.ProxyURL
				}
				if e.Weight != nil {
					km["weight"] = *e.Weight
				}
				keys = append(keys, km)
			}
			out["api-key-entries"] = keys
		}
		if len(p.Headers) > 0 {
			out["headers"] = p.Headers
		}
	}

	if p.Priority != 0 {
		out["priority"] = p.Priority
	} else {
		delete(out, "priority")
	}

	models := make([]map[string]any, 0, len(p.Models))
	for _, m := range p.Models {
		models = append(models, m.ToMap())
	}
	out["models"] = models
	return out
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// CloneProvider deep-copies the parts of a provider the panel mutates.
func CloneProvider(p Provider) Provider {
	cp := p
	cp.Raw = cloneMap(p.Raw)
	if p.Headers != nil {
		cp.Headers = make(map[string]string, len(p.Headers))
		for k, v := range p.Headers {
			cp.Headers[k] = v
		}
	}
	if p.APIKeyEntries != nil {
		cp.APIKeyEntries = append([]APIKeyEntry(nil), p.APIKeyEntries...)
	}
	if p.Models != nil {
		cp.Models = make([]Model, len(p.Models))
		for i, m := range p.Models {
			cm := m
			cm.Raw = cloneMap(m.Raw)
			cp.Models[i] = cm
		}
	}
	return cp
}
