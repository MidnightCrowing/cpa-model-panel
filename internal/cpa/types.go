package cpa

// Channel is one of CPA's three provider configuration lists. A model's
// channel is its physical location in CPA's config and is never inferred —
// the panel always writes a model back to the channel it was read from.
type Channel string

const (
	ChannelOpenAI Channel = "openai"
	ChannelCodex  Channel = "codex"
	ChannelClaude Channel = "claude"
)

// AllChannels is the canonical iteration order.
var AllChannels = []Channel{ChannelOpenAI, ChannelCodex, ChannelClaude}

// path returns the management endpoint suffix for the channel.
func (c Channel) path() string {
	switch c {
	case ChannelCodex:
		return "codex-api-key"
	case ChannelClaude:
		return "claude-api-key"
	default:
		return "openai-compatibility"
	}
}

// jsonKey returns the wrapper object key CPA uses in GET responses.
func (c Channel) jsonKey() string { return c.path() }

func (c Channel) Valid() bool {
	return c == ChannelOpenAI || c == ChannelCodex || c == ChannelClaude
}

// Provider is one entry of a channel list.
//
// Raw holds the untouched JSON object so every field the panel does not
// understand (auth-index, proxy-url, custom flags…) survives a round trip.
type Provider struct {
	Name                  string
	Priority              int
	Disabled              bool
	Prefix                string
	BaseURL               string
	APIKeyEntries         []APIKeyEntry
	Models                []Model
	Headers               map[string]string
	SupportPromptCacheKey bool
	DisableCooling        bool

	Raw map[string]any
}

type APIKeyEntry struct {
	APIKey   string
	Weight   *int
	ProxyURL string
}

// APIKeys returns every API key configured on the provider, in order.
func (p Provider) APIKeys() []string {
	out := make([]string, 0, len(p.APIKeyEntries))
	for _, e := range p.APIKeyEntries {
		if e.APIKey != "" {
			out = append(out, e.APIKey)
		}
	}
	return out
}

// Model is one entry of a provider's models list. Raw preserves the original
// object; only Alias is ever rewritten by the panel.
type Model struct {
	Name  string
	Alias string

	Raw map[string]any
}
