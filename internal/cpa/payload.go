package cpa

import "encoding/json"

// ProvidersFromPayload rebuilds providers from a stored snapshot payload.
func ProvidersFromPayload(payload []map[string]any) ([]Provider, error) {
	out := make([]Provider, 0, len(payload))
	for _, item := range payload {
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		p, err := parseProvider(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
