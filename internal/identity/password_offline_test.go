//go:build password_rehearsal

package identity

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPasswordPBKDF2FixtureVector(t *testing.T) {
	salt := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	s := Passwords{hashing: make(chan struct{}, 2)}
	hash, err := s.hash(context.Background(), "first test password with spaces", salt)
	if err != nil || hex.EncodeToString(hash) != "a96b8f9464ae8a56cccff32d7ddd5828e97f456052a4a07ebcd352f6c0bd3e93" {
		t.Fatal("password scheme does not match independent fixture vector")
	}
}

func TestPasswordDeliveryChecksTargetPurposeAndCiphertext(t *testing.T) {
	s, err := NewPasswords(&Lifecycle{Pool: &pgxpool.Pool{}}, bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal("fixture delivery initialization failed")
	}
	token, _ := RandomToken()
	if token == "" {
		t.Fatal("fixture token unavailable")
	}
	hash := TokenHash(token)
	payload := s.delivery.Seal(nil, nil, []byte(token), deliveryAAD("fixture", hash, "user", "invite"))
	opened, err := s.OpenDelivery("fixture", hash, "user", "invite", payload)
	if err != nil || opened != token || len(payload) != 92 {
		t.Fatal("encrypted fixture did not round trip")
	}
	for _, claim := range [][2]string{{"other-user", "invite"}, {"user", "reset"}} {
		if _, err = s.OpenDelivery("fixture", hash, claim[0], claim[1], payload); !errors.Is(err, ErrUnauthenticated) {
			t.Fatal("delivery accepted a different target or purpose")
		}
	}
	payload[len(payload)-1] ^= 1
	if _, err = s.OpenDelivery("fixture", hash, "user", "invite", payload); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("delivery accepted altered ciphertext")
	}
}
