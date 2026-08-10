package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/local/cpa-model-panel/internal/cpa"
)

// Fingerprint hashes the exact payload the panel would write for the current
// CPA configuration. Any external edit — a model, a key, a priority, a whole
// provider — changes it, so an optimistic-lock failure is never a false alarm
// and never a missed one.
//
// The previous implementation hashed a protocol-filtered projection computed
// with the *default* regexes while saving used the *configured* ones, which
// made every save fail with 409 once the user customised a pattern.
func Fingerprint(snap *cpa.Snapshot) (string, error) {
	payload := make([][]map[string]any, 0, len(cpa.AllChannels))
	for _, ch := range cpa.AllChannels {
		payload = append(payload, cpa.ChannelPayload(snap.Providers(ch)))
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
