// tls-fixture creates a short-lived loopback certificate only inside a retained
// synthetic QA directory. It is never a certificate-management facility.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"lmsv3/internal/migration/backup"
)

func main() {
	root := os.Getenv("QA_LAB_DIR")
	if os.Getenv("QA_PG_ACK") != "new-local-synthetic-cluster" || !backup.SafeRoot(root) {
		os.Exit(1)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		os.Exit(1)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: "lms-v3-local-qa"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		os.Exit(1)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		os.Exit(1)
	}
	if backup.WriteNew(filepath.Join(root, "runtime-tls.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) != nil || backup.WriteNew(filepath.Join(root, "runtime-tls.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})) != nil {
		os.Exit(1)
	}
}
