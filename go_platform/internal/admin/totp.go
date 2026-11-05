package admin

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

func verifyTOTP(secretHex, code string, step int, t0 int64, digits, tolerance, drift int, lastT int64, now time.Time) (int64, bool) {
	if step < 15 || step > 120 || digits < 6 || digits > 8 || tolerance < 0 || tolerance > 2 || len(code) != digits {
		return 0, false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	secret, err := hex.DecodeString(secretHex)
	if err != nil || len(secret) < 10 {
		return 0, false
	}
	base := (now.Unix()-t0)/int64(step) + int64(drift)
	for offset := -tolerance; offset <= tolerance; offset++ {
		counter := base + int64(offset)
		if counter < 0 || counter <= lastT {
			continue
		}
		var message [8]byte
		binary.BigEndian.PutUint64(message[:], uint64(counter))
		mac := hmac.New(sha1.New, secret)
		_, _ = mac.Write(message[:])
		digest := mac.Sum(nil)
		position := int(digest[len(digest)-1] & 0x0f)
		number := binary.BigEndian.Uint32(digest[position:position+4]) & 0x7fffffff
		mod := uint32(1)
		for i := 0; i < digits; i++ {
			mod *= 10
		}
		candidate := fmt.Sprintf("%0*d", digits, number%mod)
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 {
			return counter, true
		}
	}
	return 0, false
}

func validTOTPInput(code string) bool {
	if len(code) < 6 || len(code) > 8 {
		return false
	}
	_, err := strconv.Atoi(code)
	return err == nil
}
