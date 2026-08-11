package clean

import (
	"regexp"
	"sort"
	"strings"
)

// DefaultPrefixes are conservative upstream noise prefixes.
var DefaultPrefixes = []string{
	"[free]",
	"@cf/",
	"@hf/",
	"@together/",
	"deepseek-ai/",
	"Qwen/",
	"google/",
	"BAAI/",
	"meta-llama/",
	"mistralai/",
	"openai/",
	"Pro/",
	"THUDM/",
	"01-ai/",
	"nvidia/",
	"ibm/",
	"llava-hf/",
	"HuggingFaceH4/",
	"huggingface/",
	"black-forest-labs/",
	"stabilityai/",
	"ai4bharat/",
	"aisingapore/",
	"defog/",
	"lykon/",
	"leonardo/",
	"deepgram/",
	"anthropic/",
	"x-ai/",
	"moonshotai/",
	"z-ai/",
}

// DefaultSuffixes are conservative upstream noise suffixes.
var DefaultSuffixes = []string{
	":free",
	"-free",
	":latest",
	"-latest",
}

// Rewrite is a regex substitution applied to the cleaned name. It exists
// because normalisation is vendor-specific: "gpt-5-5" means gpt-5.5, while
// "claude-haiku-4-5" is spelled with dashes by Anthropic and must stay that
// way. A rule per family says so explicitly instead of guessing.
type Rewrite struct {
	Pattern string `json:"pattern"`
	Replace string `json:"replace"`
}

// RulesConfig is the user-facing cleaning configuration.
type RulesConfig struct {
	Prefixes []string `json:"prefixes"`
	Suffixes []string `json:"suffixes"`
	// Protect stops suffix stripping once the name matches. "-max" is noise on
	// gpt-5.6-luna-max but part of the official name of qwen3-max, and this is
	// how that difference gets stated.
	Protect  string    `json:"protect"`
	Rewrites []Rewrite `json:"rewrites"`
}

// Rules is a compiled ruleset, pre-sorted longest-first so the most specific
// prefix and suffix win.
type Rules struct {
	prefixes []string
	suffixes []string
	protect  *regexp.Regexp
	rewrites []compiledRewrite
}

type compiledRewrite struct {
	pattern *regexp.Regexp
	replace string
}

func NewRules(cfg RulesConfig) (Rules, error) {
	prefixes := cfg.Prefixes
	if len(prefixes) == 0 {
		prefixes = DefaultPrefixes
	}
	suffixes := cfg.Suffixes
	if len(suffixes) == 0 {
		suffixes = DefaultSuffixes
	}

	r := Rules{
		prefixes: append([]string(nil), prefixes...),
		suffixes: append([]string(nil), suffixes...),
	}
	sort.SliceStable(r.prefixes, func(i, j int) bool { return len(r.prefixes[i]) > len(r.prefixes[j]) })
	sort.SliceStable(r.suffixes, func(i, j int) bool { return len(r.suffixes[i]) > len(r.suffixes[j]) })

	if pattern := strings.TrimSpace(cfg.Protect); pattern != "" {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return Rules{}, err
		}
		r.protect = compiled
	}
	for _, rule := range cfg.Rewrites {
		pattern := strings.TrimSpace(rule.Pattern)
		if pattern == "" {
			continue
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return Rules{}, err
		}
		r.rewrites = append(r.rewrites, compiledRewrite{pattern: compiled, replace: rule.Replace})
	}
	return r, nil
}

// Clean normalises an upstream model name into the suggested canonical name.
func (r Rules) Clean(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}

	s = r.stripPrefixes(s)

	// Keep only the final path component: "vendor/model" → "model".
	if i := strings.LastIndex(s, "/"); i >= 0 && i+1 < len(s) {
		s = s[i+1:]
	}

	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.Trim(s, "-/")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}

	s = r.stripSuffixes(s)

	for _, rule := range r.rewrites {
		s = rule.pattern.ReplaceAllString(s, rule.replace)
	}
	return s
}

func (r Rules) stripPrefixes(s string) string {
	for {
		lower := strings.ToLower(s)
		matched := false
		for _, p := range r.prefixes {
			pl := strings.ToLower(strings.TrimSpace(p))
			if pl == "" || len(s) <= len(pl) {
				continue
			}
			if strings.HasPrefix(lower, pl) {
				s = strings.TrimSpace(s[len(pl):])
				matched = true
				break
			}
		}
		if !matched {
			return s
		}
	}
}

// stripSuffixes peels noise off the end, one suffix at a time, and stops as
// soon as the remaining name is protected. Checking after every step is what
// lets "qwen3.8-max-preview" lose "-preview" and keep "-max".
func (r Rules) stripSuffixes(s string) string {
	for {
		if r.protect != nil && r.protect.MatchString(s) {
			return s
		}
		lower := strings.ToLower(s)
		matched := false
		for _, suf := range r.suffixes {
			sl := strings.ToLower(strings.TrimSpace(suf))
			if sl == "" || len(s) <= len(sl) {
				continue
			}
			if strings.HasSuffix(lower, sl) {
				s = strings.Trim(strings.TrimSpace(s[:len(s)-len(sl)]), "-/")
				matched = true
				break
			}
		}
		if !matched {
			return s
		}
	}
}
