package license

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

const kitPayload = `{"product":"kit","sub":"customer","tier":"commercial","iat":"2026-11-20T00:00:00Z","exp":"2026-12-21T00:00:00Z","features":["workspaces"],"limits":{"runs":10}}`
const devFooter = `{"kid":"hk-test-dev"}`

func signed(payload, footer string, signer ed25519.PrivateKey) string {
	var preauth bytes.Buffer
	preauth.Write([]byte{4, 0, 0, 0, 0, 0, 0, 0})
	for _, piece := range []string{"v4.public.", payload, footer, ""} {
		_ = binary.Write(&preauth, binary.LittleEndian, uint64(len(piece)))
		preauth.WriteString(piece)
	}
	sig := ed25519.Sign(signer, preauth.Bytes())
	return "v4.public." + base64.RawURLEncoding.EncodeToString(append([]byte(payload), sig...)) +
		"." + base64.RawURLEncoding.EncodeToString([]byte(footer))
}

func TestVerificationRequiresKitProductAndTrustedSignature(t *testing.T) {
	t.Setenv("HEV_LICENSE_TEST_PUBLIC_KEY_B64", "")
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	token := signed(kitPayload, devFooter, key)
	claims, err := Development().Verify(token)
	if err != nil || claims.Product != "kit" || claims.Subject != "customer" || claims.Limits["runs"] != 10 {
		t.Fatalf("verified Kit claims were not kept: %v", err)
	}
	prod := Production()
	ceremony := prod.keys["hk-2026a"]
	if len(prod.keys) != 1 || hex.EncodeToString(ceremony[:]) != "9a2b0697fecf6912b722a80f17667bbe19fbcdf2267305880f66fb9e58f7850d" {
		t.Fatal("production trust does not match the Kit ceremony")
	}
	if _, err := prod.Verify(token); err == nil {
		t.Fatal("production trusted the development key")
	}
	layerToken := signed(kitPayload, `{"kid":"hl-2026a"}`, key)
	var wrongProduct *WrongProductKidError
	if _, err := prod.Verify(layerToken); !errors.As(err, &wrongProduct) || !strings.Contains(err.Error(), "HEV_LICENSE") {
		t.Fatal("Layer key should fail with a Kit configuration diagnostic")
	}
	bodyPart, footerPart, _ := strings.Cut(strings.TrimPrefix(token, header), ".")
	body, _ := base64.RawURLEncoding.DecodeString(bodyPart)
	body[10] ^= 1
	for name, invalid := range map[string]string{
		"wrong product":    signed(strings.Replace(kitPayload, `"kit"`, `"layer"`, 1), devFooter, key),
		"missing product":  signed(strings.Replace(kitPayload, `"product":"kit",`, "", 1), devFooter, key),
		"unknown kid":      signed(kitPayload, `{"kid":"unknown"}`, key),
		"wrong purpose":    strings.Replace(token, "public", "local", 1),
		"wrong version":    strings.Replace(token, "v4", "v3", 1),
		"short body":       header + "AA." + footerPart,
		"tampered payload": header + base64.RawURLEncoding.EncodeToString(body) + "." + footerPart,
		"tampered footer":  header + bodyPart + "." + base64.RawURLEncoding.EncodeToString([]byte(devFooter+" ")),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Development().Verify(invalid); err == nil || strings.Contains(err.Error(), invalid) {
				t.Fatal("invalid input was accepted or leaked")
			}
		})
	}
}

func TestOverrideChangesOnlyDevelopmentTrust(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	seed[0] = 1
	key := ed25519.NewKeyFromSeed(seed)
	t.Setenv("HEV_LICENSE_TEST_PUBLIC_KEY_B64", base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)))
	if _, err := Development().Verify(signed(kitPayload, devFooter, key)); err != nil {
		t.Fatal(err)
	}
	for _, footer := range []string{devFooter, `{"kid":"hk-2026a"}`} {
		if _, err := Production().Verify(signed(kitPayload, footer, key)); err == nil {
			t.Fatal("production accepted overridden public key")
		}
	}
}

func TestTimelineKeepsInclusiveDeadlinesAndTrialSkipsGrace(t *testing.T) {
	exp := time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		now  time.Time
		tier string
		want Timeline
	}{
		{exp.Add(-time.Second), "trial", Timeline{State: Licensed, SecondsToDeadline: 1}},
		{exp, "trial", Timeline{State: Licensed}},
		{exp.Add(time.Second), "trial", Timeline{State: Floor}},
		{exp.Add(time.Second), "commercial", Timeline{State: Grace, SecondsToDeadline: 1209599, GraceSecondsRemaining: 1209599}},
		{exp.Add(DefaultGrace), "commercial", Timeline{State: Grace}},
		{exp.Add(DefaultGrace + time.Second), "commercial", Timeline{State: Floor}},
	} {
		if got := Evaluate(tc.now, exp, tc.tier, DefaultGrace); got != tc.want {
			t.Errorf("%s at %s: got %+v, want %+v", tc.tier, tc.now, got, tc.want)
		}
	}
	if ClampGrace(time.Hour) != time.Hour || ClampGrace(-time.Hour) != 0 || ClampGrace(2*DefaultGrace) != DefaultGrace {
		t.Fatal("grace was not clamped to the supported range")
	}
}
