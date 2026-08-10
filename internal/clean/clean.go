package clean

import (
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

// Rules is the configured cleaning ruleset, pre-sorted longest-first so the
// most specific prefix/suffix wins.
type Rules struct {
	prefixes []string
	suffixes []string
}

func NewRules(prefixes, suffixes []string) Rules {
	if len(prefixes) == 0 {
		prefixes = DefaultPrefixes
	}
	if len(suffixes) == 0 {
		suffixes = DefaultSuffixes
	}
	r := Rules{
		prefixes: append([]string(nil), prefixes...),
		suffixes: append([]string(nil), suffixes...),
	}
	sort.SliceStable(r.prefixes, func(i, j int) bool { return len(r.prefixes[i]) > len(r.prefixes[j]) })
	sort.SliceStable(r.suffixes, func(i, j int) bool { return len(r.suffixes[i]) > len(r.suffixes[j]) })
	return r
}

// Clean normalises an upstream model name into the suggested canonical name.
func (r Rules) Clean(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}

	s = stripPrefixes(s, r.prefixes)

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

	return stripSuffixes(s, r.suffixes)
}

func stripPrefixes(s string, prefixes []string) string {
	for {
		lower := strings.ToLower(s)
		matched := false
		for _, p := range prefixes {
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

func stripSuffixes(s string, suffixes []string) string {
	for {
		lower := strings.ToLower(s)
		matched := false
		for _, suf := range suffixes {
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
