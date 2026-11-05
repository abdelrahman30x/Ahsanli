package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"strconv"
	"strings"
)

// Django's default pbkdf2_sha256 format is algorithm$iterations$salt$hash.
func verifyDjangoPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" || len(parts[2]) < 8 {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 10000 || iterations > 2_000_000 {
		return false
	}
	expected, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) != sha256.Size {
		return false
	}
	actual := pbkdf2SHA256([]byte(password), []byte(parts[2]), iterations)
	return subtle.ConstantTimeCompare(actual[:], expected) == 1
}

func pbkdf2SHA256(password, salt []byte, iterations int) [sha256.Size]byte {
	var block [4]byte
	binary.BigEndian.PutUint32(block[:], 1)
	mac := hmac.New(sha256.New, password)
	_, _ = mac.Write(salt)
	_, _ = mac.Write(block[:])
	var u [sha256.Size]byte
	copy(u[:], mac.Sum(nil))
	out := u
	for i := 1; i < iterations; i++ {
		mac.Reset()
		_, _ = mac.Write(u[:])
		copy(u[:], mac.Sum(nil))
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}
