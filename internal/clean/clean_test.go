package clean

import "testing"

func TestCleanStripsPrefixesSuffixesAndPaths(t *testing.T) {
	rules := NewRules(nil, nil)
	cases := map[string]string{
		"[free]claude-opus-4-6":   "claude-opus-4-6",
		"deepseek-ai/DeepSeek-V3": "deepseek-v3",
		"openai/gpt-5.4":          "gpt-5.4",
		"Qwen/Qwen3-32B:free":     "qwen3-32b",
		"@cf/meta/llama-3-8b":     "llama-3-8b",
		"glm_4.5":                 "glm-4.5",
		"gpt-4o-latest":           "gpt-4o",
	}
	for in, want := range cases {
		if got := rules.Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanNeverEmptiesAName(t *testing.T) {
	rules := NewRules([]string{"gpt"}, []string{"-5"})
	if got := rules.Clean("gpt-5"); got == "" {
		t.Fatal("cleaning consumed the whole name")
	}
}

// The version number has to belong to the series: at most one word may sit
// between them, and a stray number deeper in the name is not a version.
// "grok-code-fast-1" is a coder model, not grok 1.
func TestExtractVersionOnlyReadsNumbersBelongingToTheSeries(t *testing.T) {
	cases := []struct {
		name    string
		series  string
		want    float64
		present bool
	}{
		{"gpt-4o", "gpt", 4, true},
		{"gpt-5.4", "gpt", 5.4, true},
		{"gpt-5.4-mini", "gpt", 5.4, true},
		{"claude-3-7-sonnet", "claude", 3.7, true},
		{"claude-opus-4-6", "claude", 4.6, true},
		{"gemini-2.5-pro", "gemini", 2.5, true},
		{"grok-4-fast", "grok", 4, true},
		{"kimi-k2", "kimi", 2, true},
		{"minimax-m2", "minimax", 2, true},
		{"deepseek-v3.2", "deepseek", 3.2, true},
		{"doubao-seed-1.6", "doubao", 1.6, true},
		{"grok-code-fast-1", "grok", 0, false},
		{"deepseek-v3", "gpt", 0, false},
	}
	for _, c := range cases {
		got, ok := ExtractVersion(c.name, c.series)
		if ok != c.present || (ok && got != c.want) {
			t.Errorf("ExtractVersion(%q, %q) = %v, %v; want %v, %v", c.name, c.series, got, ok, c.want, c.present)
		}
	}
}

func TestEvaluateHonoursExemptionsAndThresholds(t *testing.T) {
	cfg := VersionFilterConfig{
		Enabled:       true,
		Thresholds:    []SeriesThreshold{{Series: "gpt", MinVersion: 5}, {Series: "claude", MinVersion: 4}},
		ExemptPattern: `(embed|chat)`,
	}
	if v := Evaluate(cfg, "gpt-4o"); v.Keep {
		t.Error("gpt-4o should be dropped below the gpt threshold")
	}
	if v := Evaluate(cfg, "gpt-5.4"); !v.Keep {
		t.Error("gpt-5.4 should be kept")
	}
	if v := Evaluate(cfg, "chatgpt-4o-latest"); !v.Keep || !v.Exempt {
		t.Error("exempt pattern should win over the threshold")
	}
	if v := Evaluate(cfg, "llama-3"); !v.Keep {
		t.Error("models of unlisted series must be kept")
	}
	cfg.Enabled = false
	if v := Evaluate(cfg, "gpt-4o"); !v.Keep {
		t.Error("disabled filter must keep everything")
	}
}

func TestProtocolMatcherClassifies(t *testing.T) {
	matcher, err := NewProtocolMatcher(DefaultProtocolConfig())
	if err != nil {
		t.Fatalf("NewProtocolMatcher: %v", err)
	}
	cases := map[string]string{
		"openai/gpt-5.4":            ProtocolCodex,
		"anthropic/claude-opus-4.5": ProtocolClaude,
		"deepseek-v3":               ProtocolOpenAI,
	}
	for name, want := range cases {
		if got := matcher.Classify(name); got != want {
			t.Errorf("Classify(%q) = %q, want %q", name, got, want)
		}
	}

	// The alias is considered too, so a site that prefixes everything still
	// classifies correctly.
	if got := matcher.Classify("model-17", "gpt-5.4"); got != ProtocolCodex {
		t.Errorf("Classify with alias = %q, want codex", got)
	}
}

func TestProtocolMatcherRejectsBadPattern(t *testing.T) {
	if _, err := NewProtocolMatcher(ProtocolConfig{CodexRegex: "("}); err == nil {
		t.Fatal("expected an error for an invalid regex")
	}
}

// RE2 has no lookahead. The pattern is used exactly as written, so an
// unsupported one is an error the user sees — not a silent fallback to some
// built-in default, which is how the old panel hid the problem for weeks.
func TestProtocolRegexIsUsedVerbatim(t *testing.T) {
	if _, err := NewProtocolMatcher(ProtocolConfig{CodexRegex: `(?i)^(?!.*mini).*gpt.*`}); err == nil {
		t.Fatal("expected RE2 to reject a lookahead pattern")
	}

	// The RE2 way to say "gpt models but not the mini/image variants" is to
	// describe what does qualify.
	matcher, err := NewProtocolMatcher(ProtocolConfig{
		CodexRegex: `(?i)^(?:[\w.-]+/)?gpt-\d+(?:[.-]\d+)*(?:-(?:luna|sol|terra|pro20x|pro|openai-compact|compact|max|fast|xhigh|high|medium|low))*(?:\((?:xhigh|high|medium|low)\))?(?:\[1M\])?$`,
	})
	if err != nil {
		t.Fatalf("NewProtocolMatcher: %v", err)
	}
	codex := []string{"gpt-5.4", "gpt-5.4(xhigh)[1M]", "openai/gpt-5.6-luna", "gpt-5.4-2026-03-05", "gpt-5.5-pro20x"}
	for _, name := range codex {
		if got := matcher.Classify(name); got != ProtocolCodex {
			t.Errorf("Classify(%q) = %q, want codex", name, got)
		}
	}
	others := []string{"gpt-5.4-mini", "gpt-image-2", "gpt-oss-120b", "gpt-5-5-chat", "openai/gpt-audio", "openai/gpt-5.4-nano"}
	for _, name := range others {
		if got := matcher.Classify(name); got != ProtocolOpenAI {
			t.Errorf("Classify(%q) = %q, want openai", name, got)
		}
	}
}

// An empty pattern tags nothing; it must not resurrect the built-in default.
func TestEmptyProtocolPatternMatchesNothing(t *testing.T) {
	matcher, err := NewProtocolMatcher(ProtocolConfig{})
	if err != nil {
		t.Fatalf("NewProtocolMatcher: %v", err)
	}
	if got := matcher.Classify("gpt-5.4"); got != ProtocolOpenAI {
		t.Errorf("Classify with no patterns = %q, want openai", got)
	}
}
