// bridge-fixture is an offline synthetic issuer, not an application endpoint.
// It cannot read dotenv or contact any provider. The application receives only
// its public key; the private seed remains in retained local QA artifacts.
package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"github.com/golang-jwt/jwt/v5"
	"io"
	"lmsv3/internal/migration/backup"
	"os"
	"path/filepath"
	"time"
)

func main() {
	root := os.Getenv("QA_LAB_DIR")
	if os.Getenv("QA_PG_ACK") != "new-local-synthetic-cluster" || !backup.SafeRoot(root) {
		os.Exit(1)
	}
	data, err := os.ReadFile(filepath.Join(root, "issuer-seed.private"))
	if err != nil {
		os.Exit(1)
	}
	seed, err := hex.DecodeString(string(data))
	if err != nil || len(seed) != ed25519.SeedSize {
		os.Exit(1)
	}
	claims := jwt.MapClaims{}
	d := json.NewDecoder(io.LimitReader(os.Stdin, 8193))
	if d.Decode(&claims) != nil {
		os.Exit(1)
	}
	now := time.Now().Unix()
	for k, v := range map[string]any{"iss": "https://synthetic-issuer.invalid", "aud": "lms-v3-local", "iat": now, "nbf": now, "exp": now + 60, "purpose": "login", "version": 1, "active": true} {
		if _, ok := claims[k]; !ok {
			claims[k] = v
		}
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = "fixture-1"
	token.Header["typ"] = "lms-bridge+jwt"
	raw, err := token.SignedString(ed25519.NewKeyFromSeed(seed))
	if err != nil {
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"assertion": raw})
}
