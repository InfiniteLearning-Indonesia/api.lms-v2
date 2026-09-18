package files

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const MaxBytes = 2 << 20

var objectKeyPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type ObjectStore interface {
	PutQuarantine(string, string, []byte) error
	Promote(string, string) error
	Read(string, string) ([]byte, error)
	ReadQuarantine(string, string) ([]byte, error)
}

type Scanner interface {
	Scan(context.Context, []byte) error
}

var ErrScanUnavailable = errors.New("SCAN_UNAVAILABLE")
var ErrThreat = errors.New("THREAT_DETECTED")

// FixtureScanner is deterministic and has no network/provider access. A real
// scanner adapter remains an external M06 gate.
type FixtureScanner struct{}

func (FixtureScanner) Scan(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return ErrScanUnavailable
	}
	if bytes.Contains(data, []byte("X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR")) {
		return ErrThreat
	}
	if bytes.HasPrefix(data, []byte("SCAN_UNAVAILABLE")) {
		return ErrScanUnavailable
	}
	return nil
}

type LocalStore struct{ root string }

func NewLocalStore(root string) (*LocalStore, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("PRIVATE_STORE_ROOT_REQUIRED")
	}
	root = filepath.Clean(root)
	if err := secureDirectory(root); err != nil {
		return nil, err
	}
	for _, name := range []string{"quarantine", "objects"} {
		if err := secureDirectory(filepath.Join(root, name)); err != nil {
			return nil, err
		}
	}
	return &LocalStore{root: root}, nil
}

func secureDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("PRIVATE_STORE_PATH_REJECTED")
	}
	return os.Chmod(path, 0700)
}

func (s *LocalStore) path(area, key string) (string, error) {
	if !objectKeyPattern.MatchString(key) || (area != "quarantine" && area != "objects") {
		return "", errors.New("OBJECT_KEY_REJECTED")
	}
	directory := filepath.Join(s.root, area, key[:2])
	if err := secureDirectory(directory); err != nil {
		return "", err
	}
	path := filepath.Join(directory, key)
	if filepath.Dir(path) != directory {
		return "", errors.New("OBJECT_PATH_REJECTED")
	}
	return path, nil
}

func verifyFile(path, expectedDigest string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 1 || info.Size() > MaxBytes {
		return errors.New("OBJECT_FILE_REJECTED")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, io.LimitReader(file, MaxBytes+1)); err != nil || hex.EncodeToString(hash.Sum(nil)) != expectedDigest {
		return errors.New("OBJECT_INTEGRITY_REJECTED")
	}
	return nil
}

func (s *LocalStore) PutQuarantine(key, expectedDigest string, data []byte) error {
	if len(data) < 1 || len(data) > MaxBytes {
		return errors.New("OBJECT_BYTES_REJECTED")
	}
	if !objectKeyPattern.MatchString(expectedDigest) {
		return errors.New("OBJECT_DIGEST_REJECTED")
	}
	objectPath, err := s.path("objects", key)
	if err != nil {
		return err
	}
	if verifyFile(objectPath, expectedDigest) == nil {
		return nil
	}
	path, err := s.path("quarantine", key)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return verifyFile(path, expectedDigest)
	}
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("OBJECT_WRITE_FAILED")
	}
	return verifyFile(path, expectedDigest)
}

func (s *LocalStore) Promote(key, expectedDigest string) error {
	source, err := s.path("quarantine", key)
	if err != nil {
		return err
	}
	target, err := s.path("objects", key)
	if err != nil {
		return err
	}
	if err = os.Rename(source, target); err != nil {
		if verifyFile(target, expectedDigest) == nil {
			return nil
		}
		return err
	}
	return verifyFile(target, expectedDigest)
}

func (s *LocalStore) Read(key, expectedDigest string) ([]byte, error) {
	path, err := s.path("objects", key)
	if err != nil {
		return nil, err
	}
	if err = verifyFile(path, expectedDigest); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s *LocalStore) ReadQuarantine(key, expectedDigest string) ([]byte, error) {
	path, err := s.path("quarantine", key)
	if err != nil {
		return nil, err
	}
	if err = verifyFile(path, expectedDigest); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
