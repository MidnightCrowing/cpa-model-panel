package clean

import (
	"regexp"
	"strings"
)

// Protocol labels. A protocol is a *display* classification derived from the
// model name; it never decides which CPA channel an existing model is written
// back to. It only picks the target list for a newly discovered model.
const (
	ProtocolOpenAI = "openai"
	ProtocolCodex  = "codex"
	ProtocolClaude = "claude"
)

// ProtocolConfig holds one pattern per protocol.
//
// The patterns are used exactly as written. An empty pattern tags nothing, and
// a pattern that does not compile is an error — never a silent fallback to
// some built-in default.
type ProtocolConfig struct {
	CodexRegex  string `json:"codex_regex"`
	ClaudeRegex string `json:"claude_regex"`
}

// Seed values for a fresh install. They are written into the settings on first
// read and are editable like anything else; nothing re-applies them later.
const (
	DefaultCodexRegex  = `(?i)gpt`
	DefaultClaudeRegex = `(?i)claude`
)

func DefaultProtocolConfig() ProtocolConfig {
	return ProtocolConfig{CodexRegex: DefaultCodexRegex, ClaudeRegex: DefaultClaudeRegex}
}

// ProtocolMatcher compiles the patterns once, instead of once per model as the
// previous implementation did.
type ProtocolMatcher struct {
	codex  *regexp.Regexp
	claude *regexp.Regexp
}

func NewProtocolMatcher(cfg ProtocolConfig) (*ProtocolMatcher, error) {
	codex, err := compile(cfg.CodexRegex)
	if err != nil {
		return nil, err
	}
	claude, err := compile(cfg.ClaudeRegex)
	if err != nil {
		return nil, err
	}
	return &ProtocolMatcher{codex: codex, claude: claude}, nil
}

func compile(pattern string) (*regexp.Regexp, error) {
	value := strings.TrimSpace(pattern)
	if value == "" {
		return nil, nil
	}
	return regexp.Compile(value)
}

// Classify returns the protocol for a model. Codex wins over Claude when both
// match, which is the historical behaviour.
func (m *ProtocolMatcher) Classify(names ...string) string {
	if anyMatch(m.codex, names) {
		return ProtocolCodex
	}
	if anyMatch(m.claude, names) {
		return ProtocolClaude
	}
	return ProtocolOpenAI
}

func anyMatch(pattern *regexp.Regexp, names []string) bool {
	if pattern == nil {
		return false
	}
	for _, name := range names {
		if strings.TrimSpace(name) != "" && pattern.MatchString(name) {
			return true
		}
	}
	return false
}
