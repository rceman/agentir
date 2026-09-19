package source

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256 returns a lowercase hex SHA-256 digest.
func SHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
