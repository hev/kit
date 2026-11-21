// Package license verifies Kit Pro licenses and exposes local entitlement.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

const header = "v4.public."
const maxTokenSize = 64 << 10

type KeySet struct {
	keys map[string][ed25519.PublicKeySize]byte
}

func Production() KeySet {
	return KeySet{keys: map[string][32]byte{
		"hk-2026a": {0x9a, 0x2b, 0x06, 0x97, 0xfe, 0xcf, 0x69, 0x12, 0xb7, 0x22, 0xa8, 0x0f, 0x17, 0x66, 0x7b, 0xbe, 0x19, 0xfb, 0xcd, 0xf2, 0x26, 0x73, 0x05, 0x88, 0x0f, 0x66, 0xfb, 0x9e, 0x58, 0xf7, 0x85, 0x0d},
	}}
}

func Development() KeySet {
	set := Production()
	key := [32]byte{59, 106, 39, 188, 206, 182, 164, 45, 98, 163, 168, 208, 42, 111, 13, 115, 101, 50, 21, 119, 29, 226, 67, 166, 58, 192, 72, 161, 139, 89, 218, 41}
	if override := os.Getenv("HEV_LICENSE_TEST_PUBLIC_KEY_B64"); override != "" {
		data, err := base64.StdEncoding.DecodeString(override)
		if err != nil || len(data) != len(key) {
			return set
		}
		copy(key[:], data)
	}
	set.keys["hk-test-dev"] = key
	return set
}

type Claims struct {
	Product  string            `json:"product"`
	Subject  string            `json:"sub"`
	Tier     string            `json:"tier"`
	JTI      string            `json:"jti,omitempty"`
	Features []string          `json:"features"`
	Limits   map[string]uint64 `json:"limits"`
	IssuedAt time.Time         `json:"iat"`
	Expires  time.Time         `json:"exp"`
}

type WrongProductKidError struct{}

func (*WrongProductKidError) Error() string {
	return "license token uses hev layer key id hl-*; hev kit pro requires a hev kit license (HEV_LICENSE)"
}

func pae(pieces ...[]byte) []byte {
	out := binary.LittleEndian.AppendUint64(nil, uint64(len(pieces)))
	for _, piece := range pieces {
		out = binary.LittleEndian.AppendUint64(out, uint64(len(piece)))
		out = append(out, piece...)
	}
	return out
}

func (set KeySet) Verify(token string) (Claims, error) {
	invalid := errors.New("invalid Kit license token")
	if len(token) > maxTokenSize || !strings.HasPrefix(token, header) || strings.ContainsAny(token, "\r\n") {
		return Claims{}, invalid
	}
	bodyPart, footerPart, ok := strings.Cut(token[len(header):], ".")
	if !ok {
		return Claims{}, invalid
	}
	encoding := base64.RawURLEncoding.Strict()
	body, err := encoding.DecodeString(bodyPart)
	if err != nil || len(body) <= ed25519.SignatureSize {
		return Claims{}, invalid
	}
	footer, err := encoding.DecodeString(footerPart)
	var keyID struct {
		Kid string `json:"kid"`
	}
	if err != nil || json.Unmarshal(footer, &keyID) != nil || keyID.Kid == "" {
		return Claims{}, invalid
	}
	if strings.HasPrefix(keyID.Kid, "hl-") {
		return Claims{}, &WrongProductKidError{}
	}
	key, trusted := set.keys[keyID.Kid]
	if !trusted {
		return Claims{}, errors.New("untrusted Kit license key id")
	}
	payload, signature := body[:len(body)-ed25519.SignatureSize], body[len(body)-ed25519.SignatureSize:]
	if !ed25519.Verify(key[:], pae([]byte(header), payload, footer, nil), signature) {
		return Claims{}, errors.New("invalid Kit license signature")
	}
	var claims Claims
	if json.Unmarshal(payload, &claims) != nil || claims.Expires.IsZero() || claims.IssuedAt.IsZero() || claims.Tier == "" || claims.Subject == "" {
		return Claims{}, errors.New("invalid Kit license claims")
	}
	if claims.Product != "kit" {
		return Claims{}, errors.New("license product must be kit")
	}
	return claims, nil
}
