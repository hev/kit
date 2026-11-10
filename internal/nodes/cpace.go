package nodes

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"math/big"
)

// A CPace-style balanced PAKE (draft-irtf-cfrg-cpace) on X25519. Both sides
// derive a password-dependent generator G = Elligator2(H(code)), exchange
// Y = y*G for fresh secret scalars y, and key on K = y_self*Y_peer. A party
// that does not know the code cannot compute K, and a recorded or
// impersonated exchange gives no equation to test code guesses against
// offline: checking a guess c' would need the discrete log of Y with respect
// to G(c'). Each online run tests at most one guess, which invite lockout
// bounds. The construction is not interoperable with the draft's test
// vectors; it only needs to agree with itself.

const cpaceDST = "hev-nodes-cpace-v1"

var (
	fieldP = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	curveJ = big.NewInt(486662)
)

func lengthPrefixed(parts ...[]byte) []byte {
	out := []byte{}
	for _, p := range parts {
		out = binary.BigEndian.AppendUint32(out, uint32(len(p)))
		out = append(out, p...)
	}
	return out
}

// cpaceGenerator maps the slot and secret to a Curve25519 u-coordinate with
// the Elligator2 map of RFC 9380 (Z = 2). math/big is not constant time; the
// map runs once per exchange on a value an attacker cannot observe, and
// process start and SSH dominate any timing signal.
func cpaceGenerator(slot, secret string) []byte {
	h := sha512.Sum512(lengthPrefixed([]byte(cpaceDST+" generator"), []byte(slot), []byte(secret)))
	u := new(big.Int).Mod(new(big.Int).SetBytes(h[:]), fieldP)
	mod := func(v *big.Int) *big.Int { return v.Mod(v, fieldP) }
	tv := mod(new(big.Int).Mul(u, u))
	tv = mod(tv.Lsh(tv, 1))
	tv = mod(tv.Add(tv, big.NewInt(1)))
	negJ := mod(new(big.Int).Neg(curveJ))
	x1 := new(big.Int).Set(negJ)
	if tv.Sign() != 0 {
		x1 = mod(x1.Mul(x1, new(big.Int).ModInverse(tv, fieldP)))
	}
	g := func(x *big.Int) *big.Int {
		x2 := mod(new(big.Int).Mul(x, x))
		v := mod(new(big.Int).Mul(x2, x))
		v.Add(v, mod(new(big.Int).Mul(curveJ, x2)))
		return mod(v.Add(v, x))
	}
	square := func(v *big.Int) bool {
		e := new(big.Int).Rsh(new(big.Int).Sub(fieldP, big.NewInt(1)), 1)
		r := new(big.Int).Exp(v, e, fieldP)
		return r.Sign() == 0 || r.Cmp(big.NewInt(1)) == 0
	}
	x := x1
	if !square(g(x1)) {
		x = mod(new(big.Int).Sub(negJ, x1))
	}
	out := make([]byte, 32)
	be := x.FillBytes(make([]byte, 32))
	for i := range out {
		out[i] = be[31-i]
	}
	return out
}

type cpace struct {
	slot string
	priv *ecdh.PrivateKey
	Y    []byte
}

func newCPace(slot, secret string) (*cpace, error) {
	g, e := ecdh.X25519().NewPublicKey(cpaceGenerator(slot, secret))
	if e != nil {
		return nil, e
	}
	priv, e := ecdh.X25519().GenerateKey(rand.Reader)
	if e != nil {
		return nil, e
	}
	y, e := priv.ECDH(g)
	if e != nil {
		return nil, e
	}
	return &cpace{slot, priv, y}, nil
}

// key derives the shared session key from the member's and host's messages.
// ECDH rejects low-order peer points (an all-zero K).
func (c *cpace) key(member, host []byte, isHost bool) ([]byte, error) {
	peer := host
	if isHost {
		peer = member
	}
	if len(peer) != 32 {
		return nil, fmt.Errorf("invalid key exchange message")
	}
	// A peer echoing our own share back learns nothing, but nothing honest
	// does that either.
	if hmac.Equal(peer, c.Y) {
		return nil, fmt.Errorf("invalid key exchange message: reflected share")
	}
	pub, e := ecdh.X25519().NewPublicKey(peer)
	if e != nil {
		return nil, e
	}
	k, e := c.priv.ECDH(pub)
	if e != nil {
		return nil, fmt.Errorf("invalid key exchange message: %w", e)
	}
	isk := sha512.Sum512(lengthPrefixed([]byte(cpaceDST+" isk"), []byte(c.slot), member, host, k))
	return isk[:], nil
}

// confirm binds a role label and payload to the session key; the labels
// differ so one side's confirmation cannot be reflected as the other's.
func confirm(isk []byte, role string, payload []byte) []byte {
	m := hmac.New(sha256.New, isk)
	m.Write(lengthPrefixed([]byte(cpaceDST+" "+role), payload))
	return m.Sum(nil)
}
