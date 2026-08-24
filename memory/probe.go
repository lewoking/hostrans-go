package memory

import (
	"crypto/rand"
	"unicode/utf16"
)

func RandomProbe() string {
	const digits = "0123456789"
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "HT0000000000"
	}
	for i := range b {
		b[i] = digits[int(b[i])%10]
	}
	return "HT" + string(b)
}

func EncodeProbe(s, encoding string) []byte {
	if encoding == "utf-16le" || encoding == "utf-16" {
		u := utf16.Encode([]rune(s))
		out := make([]byte, len(u)*2)
		for i, v := range u {
			out[i*2] = byte(v)
			out[i*2+1] = byte(v >> 8)
		}
		return out
	}
	return []byte(s)
}
