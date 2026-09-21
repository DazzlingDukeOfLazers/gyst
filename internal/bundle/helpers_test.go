package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
func jsonMarshal(v any) ([]byte, error)   { return json.Marshal(v) }
func sha256Hex(b []byte) string           { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
