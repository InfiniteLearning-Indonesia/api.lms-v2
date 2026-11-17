package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const StreamBufferBytes = 64 << 10
const MaxStreamArtifactBytes int64 = 1 << 40
const MaxStreamCheckTime = 30 * time.Minute

var streamArtifactName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
var errStreamDigest = errors.New("STREAM_DIGEST_REJECTED")

// StreamDigest is byte integrity only, not archive authenticity, encryption,
// snapshot consistency, actual restore proof or operational authority.
type StreamDigest struct {
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// DigestStream requires an explicit size/budget and a deadline. Memory does not
// grow with artifact size. Cancellation is checked before/after each read; an
// arbitrary blocking reader must itself support cancellation to interrupt IO.
func DigestStream(ctx context.Context, source io.Reader, expected, maximum int64) (StreamDigest, error) {
	if ctx == nil || source == nil || expected <= 0 || maximum < expected || maximum > MaxStreamArtifactBytes {
		return StreamDigest{}, errStreamDigest
	}
	deadline, ok := ctx.Deadline()
	remaining := time.Until(deadline)
	if !ok || ctx.Err() != nil || remaining <= 0 || remaining > MaxStreamCheckTime {
		return StreamDigest{}, errStreamDigest
	}
	hash := sha256.New()
	buffer := make([]byte, StreamBufferBytes)
	var total int64
	empty := 0
	for {
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			return StreamDigest{}, errStreamDigest
		}
		length := min(int64(len(buffer)), expected+1-total)
		n, err := source.Read(buffer[:int(length)])
		if n < 0 || n > int(length) || ctx.Err() != nil || !time.Now().Before(deadline) {
			return StreamDigest{}, errStreamDigest
		}
		if n > 0 {
			empty = 0
			total += int64(n)
			if total > expected {
				return StreamDigest{}, errStreamDigest
			}
			_, _ = hash.Write(buffer[:n])
		} else {
			empty++
		}
		if err == io.EOF {
			if total != expected {
				return StreamDigest{}, errStreamDigest
			}
			return StreamDigest{Bytes: total, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
		}
		if err != nil || empty >= 100 {
			return StreamDigest{}, errStreamDigest
		}
	}
}

// VerifyStreamFile reads a basename through an anchored os.Root and matches an
// expected receipt digest. It does not create, decrypt, copy or delete a file.
// Byte hashes do not authenticate the receipt's source or replace age/restore.
func VerifyStreamFile(ctx context.Context, directory, name string, expected, maximum int64, expectedSHA256 string) (StreamDigest, error) {
	if !streamArtifactName.MatchString(name) || !fingerprint.MatchString(expectedSHA256) || !intakeRoot(directory) {
		return StreamDigest{}, errStreamDigest
	}
	beforeRoot, err := os.Lstat(directory)
	if err != nil {
		return StreamDigest{}, errStreamDigest
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return StreamDigest{}, errStreamDigest
	}
	defer root.Close()
	anchoredRoot, err := root.Stat(".")
	if err != nil || !os.SameFile(beforeRoot, anchoredRoot) {
		return StreamDigest{}, errStreamDigest
	}
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Size() != expected || intakeReparse(filepath.Join(directory, name)) {
		return StreamDigest{}, errStreamDigest
	}
	file, err := root.Open(name)
	if err != nil {
		return StreamDigest{}, errStreamDigest
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
		return StreamDigest{}, errStreamDigest
	}
	result, err := DigestStream(ctx, file, expected, maximum)
	if err != nil || result.SHA256 != expectedSHA256 {
		return StreamDigest{}, errStreamDigest
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(opened, after) || after.Size() != opened.Size() || !after.ModTime().Equal(opened.ModTime()) {
		return StreamDigest{}, errStreamDigest
	}
	return result, nil
}
