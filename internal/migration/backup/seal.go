package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Binding is authenticated, not encrypted. It contains no credentials or PII.
// This bounded format is a local building block, not an operational approval.
type Binding struct {
	RunID, Target, Scope string
	CreatedAt            time.Time
}

func (b Binding) valid(target string, now time.Time) bool {
	return token.MatchString(b.RunID) && fingerprint.MatchString(b.Target) && b.Target == target && fingerprint.MatchString(b.Scope) && !now.Before(b.CreatedAt) && now.Sub(b.CreatedAt) <= 24*time.Hour
}

func Seal(plain, key []byte, b Binding, now time.Time) ([]byte, error) {
	if len(key) != 32 || len(plain) == 0 || len(plain) > 32<<20 || !b.valid(b.Target, now) {
		return nil, errors.New("BACKUP_SEAL_INPUT_REJECTED")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	aad, _ := json.Marshal(b)
	return gcm.Seal(nonce, nonce, plain, aad), nil
}

func Unseal(data, key []byte, b Binding, target string, now time.Time) ([]byte, error) {
	rejected := errors.New("BACKUP_AUTHENTICATION_REJECTED")
	if len(key) != 32 || len(data) > (32<<20)+64 || !b.valid(target, now) {
		return nil, rejected
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, rejected
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, rejected
	}
	if len(data) < gcm.NonceSize()+gcm.Overhead() {
		return nil, rejected
	}
	aad, _ := json.Marshal(b)
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], aad)
	if err != nil {
		return nil, rejected
	}
	return plain, nil
}

// ProtectedDestination performs actual OS checks; a caller's boolean cannot
// certify ACL/capacity. Unsupported platforms fail closed for real-data export.
func ProtectedDestination(root string, required uint64) error {
	if !SafeRoot(root) || required == 0 {
		return errors.New("BACKUP_DESTINATION_REJECTED")
	}
	free, err := AvailableBytes(root)
	if err != nil || free < required {
		return errors.New("BACKUP_CAPACITY_REJECTED")
	}
	return privateDirectory(root)
}

func WriteNew(path string, data []byte) error {
	if !SafeRoot(filepath.Dir(path)) {
		return errors.New("BACKUP_PATH_REJECTED")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	// Preserve even a partial failed artifact. Never overwrite or delete it.
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
