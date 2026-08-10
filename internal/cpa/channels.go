package cpa

import (
	"encoding/json"
	"fmt"
)

// Snapshot is the full panel-visible CPA configuration: all three channel
// lists read in one pass.
type Snapshot struct {
	Channels map[Channel][]Provider
}

func (s *Snapshot) Providers(ch Channel) []Provider {
	if s == nil {
		return nil
	}
	return s.Channels[ch]
}

// Snapshot reads every channel. Unlike the previous implementation this does
// not swallow errors: a CPA outage must surface, never render as an empty
// configuration that a later save could write back.
func (c *Client) Snapshot() (*Snapshot, error) {
	out := &Snapshot{Channels: make(map[Channel][]Provider, len(AllChannels))}
	for _, ch := range AllChannels {
		providers, err := c.GetChannel(ch)
		if err != nil {
			return nil, err
		}
		out.Channels[ch] = providers
	}
	return out, nil
}

// GetChannel reads one channel list.
func (c *Client) GetChannel(ch Channel) ([]Provider, error) {
	body, err := c.get("/v0/management/" + ch.path())
	if err != nil {
		return nil, err
	}
	var wrapped map[string][]json.RawMessage
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, fmt.Errorf("decode %s: %w", ch.path(), err)
	}
	raws := wrapped[ch.jsonKey()]
	out := make([]Provider, 0, len(raws))
	for _, raw := range raws {
		p, err := parseProvider(raw)
		if err != nil {
			return nil, fmt.Errorf("decode %s entry: %w", ch.path(), err)
		}
		out = append(out, p)
	}
	return out, nil
}

// PutChannel replaces one channel list.
func (c *Client) PutChannel(ch Channel, providers []Provider) error {
	payload := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		payload = append(payload, providerToWriteMap(p))
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.put("/v0/management/"+ch.path(), data)
}

// ChannelPayload renders what PutChannel would send. Used by tests and the
// dry-run comparison to prove a no-op save changes nothing.
func ChannelPayload(providers []Provider) []map[string]any {
	out := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		out = append(out, providerToWriteMap(p))
	}
	return out
}
