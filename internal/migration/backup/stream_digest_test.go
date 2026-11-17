package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type syntheticStream struct{ maxRead, reads int }

func (s *syntheticStream) Read(p []byte) (int, error) {
	s.maxRead = max(s.maxRead, len(p))
	s.reads++
	clear(p)
	return len(p), nil
}

type streamReaderFunc func([]byte) (int, error)

func (f streamReaderFunc) Read(p []byte) (int, error) { return f(p) }

func TestStreamDigestLargeBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	const size = (33 << 20) + 17
	source := &syntheticStream{}
	got, err := DigestStream(ctx, io.LimitReader(source, size), size, size)
	if err != nil || got.Bytes != size || source.maxRead > StreamBufferBytes || source.reads < 2 {
		t.Fatal("large stream failed or unbounded read requested")
	}
	independent := sha256.New()
	_, err = io.CopyBuffer(independent, io.LimitReader(&syntheticStream{}, size), make([]byte, 8192))
	if err != nil || got.SHA256 != hex.EncodeToString(independent.Sum(nil)) {
		t.Fatal("large streaming digest mismatch")
	}
}

func TestStreamDigestRejectsInvalidAndInterrupted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	never := streamReaderFunc(func([]byte) (int, error) { t.Fatal("invalid input reached reader"); return 0, io.EOF })
	for _, pair := range [][2]int64{{0, 100}, {-1, 100}, {10, 9}, {1, MaxStreamArtifactBytes + 1}} {
		if value, err := DigestStream(ctx, never, pair[0], pair[1]); err == nil || value.Bytes != 0 || value.SHA256 != "" {
			t.Fatal("invalid size accepted")
		}
	}
	if _, err := DigestStream(context.Background(), never, 1, 1); err == nil {
		t.Fatal("missing deadline accepted")
	}
	long, stop := context.WithTimeout(context.Background(), MaxStreamCheckTime+time.Minute)
	defer stop()
	if _, err := DigestStream(long, never, 1, 1); err == nil {
		t.Fatal("unbounded time budget accepted")
	}
	stopped, stop := context.WithCancel(ctx)
	stop()
	if _, err := DigestStream(stopped, never, 1, 1); err == nil {
		t.Fatal("canceled stream read")
	}
	for _, source := range []io.Reader{strings.NewReader("abc"), strings.NewReader("abcde"),
		streamReaderFunc(func([]byte) (int, error) { return 0, errors.New("PRIVATE_READER_ERROR") }),
		streamReaderFunc(func([]byte) (int, error) { return 0, nil }),
		streamReaderFunc(func(p []byte) (int, error) { return len(p) + 1, nil }),
	} {
		value, err := DigestStream(ctx, source, 4, 100)
		if err == nil || err.Error() != "STREAM_DIGEST_REJECTED" || value.Bytes != 0 || value.SHA256 != "" {
			t.Fatal("partial/overflow/error result accepted or leaked")
		}
	}
	mid, stop := context.WithCancel(ctx)
	defer stop()
	source := streamReaderFunc(func(p []byte) (int, error) { copy(p, "abcd"); stop(); return 4, io.EOF })
	if _, err := DigestStream(mid, source, 4, 100); err == nil {
		t.Fatal("mid-read cancellation promoted to success")
	}
}

func TestStreamFileIntegrityAndContainment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	root := t.TempDir()
	name := "database.dump"
	path := filepath.Join(root, name)
	data := []byte("synthetic archive bytes, not a database dump")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	check := func(dir, name string, size int64, hash string) error {
		_, err := VerifyStreamFile(ctx, dir, name, size, 1<<20, hash)
		return err
	}
	if err := check(root, name, int64(len(data)), Hash(data)); err != nil {
		t.Fatal("valid streamed file rejected")
	}
	for _, bad := range []string{"../database.dump", ".env", "nested/database.dump", "", "database.dump/"} {
		if check(root, bad, int64(len(data)), Hash(data)) == nil {
			t.Fatal("unsafe basename accepted")
		}
	}
	if check("relative", name, int64(len(data)), Hash(data)) == nil || check(root, name, int64(len(data)), strings.Repeat("f", 64)) == nil {
		t.Fatal("unsafe root or corrupt receipt accepted")
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(path, filepath.Join(root, "linked.dump")); err != nil {
			t.Fatal(err)
		}
		if check(root, "linked.dump", int64(len(data)), Hash(data)) == nil {
			t.Fatal("linked artifact accepted")
		}
	}
	for _, changed := range [][]byte{append([]byte(nil), data[:len(data)-1]...), append(append([]byte(nil), data...), 'x'), []byte(strings.Repeat("x", len(data)))} {
		if err := os.WriteFile(path, changed, 0600); err != nil {
			t.Fatal(err)
		}
		if check(root, name, int64(len(data)), Hash(data)) == nil {
			t.Fatal("modified, appended or truncated artifact accepted")
		}
	}
}
