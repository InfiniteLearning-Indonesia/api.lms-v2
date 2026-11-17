package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const AgeToolVersion = "v1.3.2"
const AgeBackupFormat = "age-v1-tar-binding-v1"

var errAgeStream = errors.New("AGE_STREAM_REJECTED")
var ageRecipient = regexp.MustCompile(`^age1[a-z0-9]+$`)
var moduleChecksum = regexp.MustCompile(`^h1:[a-zA-Z0-9+/]{43}=$`)
var ageSecretIdentity = regexp.MustCompile(`^AGE-SECRET-KEY-1[A-Z0-9]{20,200}$`)

type AgeTool struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	ModuleSum string `json:"module_sum"`
}

type AgeStreamRequest struct {
	Binding        Binding `json:"binding"`
	Recipient      string  `json:"recipient"`
	PlainBytes     int64   `json:"plain_bytes"`
	MaxCipherBytes int64   `json:"max_cipher_bytes"`
}

type AgeStreamReceipt struct {
	Status              string       `json:"status"`
	Format              string       `json:"format"`
	Binding             Binding      `json:"binding"`
	Plain               StreamDigest `json:"plain"`
	Cipher              StreamDigest `json:"cipher"`
	TarBytes            int64        `json:"tar_bytes"`
	ToolSHA256          string       `json:"tool_sha256"`
	AuthorizesExecution bool         `json:"authorizes_execution"`
}

type ageManifest struct {
	Version    int     `json:"version"`
	Binding    Binding `json:"binding"`
	PlainBytes int64   `json:"plain_bytes"`
}

// ValidateAgeTool validates pinned native Go build provenance before execution.
// A checksum supplied by an operator is not itself operational approval.
func ValidateAgeTool(ctx context.Context, tool AgeTool, kind string) error {
	if !validPackageContext(ctx) || (kind != "age" && kind != "age-keygen") || !fingerprint.MatchString(tool.SHA256) || !moduleChecksum.MatchString(tool.ModuleSum) {
		return errAgeStream
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	if filepath.Base(tool.Path) != kind+ext || !intakeRoot(filepath.Dir(tool.Path)) {
		return errAgeStream
	}
	info, err := os.Lstat(tool.Path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<20 || intakeReparse(tool.Path) {
		return errAgeStream
	}
	if _, err = VerifyStreamFile(ctx, filepath.Dir(tool.Path), filepath.Base(tool.Path), info.Size(), 64<<20, tool.SHA256); err != nil {
		return errAgeStream
	}
	build, err := buildinfo.ReadFile(tool.Path)
	if err != nil || build.Path != "filippo.io/age/cmd/"+kind || build.Main.Path != "filippo.io/age" || build.Main.Version != AgeToolVersion || build.Main.Sum != tool.ModuleSum || build.Main.Replace != nil {
		return errAgeStream
	}
	return nil
}

func ageEnvironment() []string {
	allowed := map[string]bool{"SYSTEMROOT": true, "WINDIR": true, "TEMP": true, "TMP": true, "TMPDIR": true}
	var env []string
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if allowed[strings.ToUpper(key)] {
			env = append(env, item)
		}
	}
	return env
}

func ageCommand(ctx context.Context, tool AgeTool, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, tool.Path, args...)
	command.Env = ageEnvironment()
	command.Dir = filepath.Dir(tool.Path)
	command.Stderr = io.Discard
	command.WaitDelay = 2 * time.Second
	return command
}

type ageReader struct {
	ctx             context.Context
	source          io.Reader
	expected, total int64
	digest          hash.Hash
	empty           int
}

func newAgeReader(ctx context.Context, source io.Reader, size int64) *ageReader {
	return &ageReader{ctx: ctx, source: source, expected: size, digest: sha256.New()}
}
func (r *ageReader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil || r.total > r.expected {
		return 0, errAgeStream
	}
	if int64(len(p)) > r.expected+1-r.total {
		p = p[:r.expected+1-r.total]
	}
	n, err := r.source.Read(p)
	if n < 0 || n > len(p) || r.ctx.Err() != nil {
		return 0, errAgeStream
	}
	r.total += int64(n)
	if r.total > r.expected {
		return 0, errAgeStream
	}
	if n > 0 {
		r.empty = 0
		r.digest.Write(p[:n])
	} else {
		r.empty++
	}
	if r.empty >= 100 || err != nil && err != io.EOF {
		return 0, errAgeStream
	}
	if err == io.EOF && r.total != r.expected {
		return 0, errAgeStream
	}
	return n, err
}
func (r *ageReader) result() StreamDigest {
	return StreamDigest{r.total, hex.EncodeToString(r.digest.Sum(nil))}
}

type ageWriter struct {
	out            io.Writer
	maximum, total int64
	digest         hash.Hash
	cancel         context.CancelFunc
}

func (w *ageWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.maximum-w.total {
		w.cancel()
		return 0, errAgeStream
	}
	n, err := w.out.Write(p)
	if n < 0 || n > len(p) {
		w.cancel()
		return 0, errAgeStream
	}
	if n > 0 {
		w.digest.Write(p[:n])
		w.total += int64(n)
	}
	if err != nil || n != len(p) {
		w.cancel()
		return n, errAgeStream
	}
	return n, nil
}
func (w *ageWriter) result() StreamDigest {
	return StreamDigest{w.total, hex.EncodeToString(w.digest.Sum(nil))}
}

type ageTarResult struct {
	plain    StreamDigest
	tarBytes int64
	err      error
}

// SealAgeStream encrypts a standard tar (binding manifest + payload) with the
// pinned age CLI. Caller owns the writer/protected storage. Partial ciphertext
// is not deleted or promoted; no receipt is returned on failure. Source readers
// must support cancellation for any blocking I/O, as with DigestStream.
func SealAgeStream(ctx context.Context, tool AgeTool, r AgeStreamRequest, source io.Reader, out io.Writer, now time.Time) (AgeStreamReceipt, error) {
	if source == nil || out == nil || !validPackageContext(ctx) || !r.Binding.valid(r.Binding.Target, now) ||
		len(r.Recipient) < 24 || len(r.Recipient) > 4100 || !ageRecipient.MatchString(r.Recipient) || r.PlainBytes <= 0 || r.PlainBytes > MaxStreamArtifactBytes-(1<<20) ||
		r.MaxCipherBytes < r.PlainBytes+65536 || r.MaxCipherBytes > MaxStreamArtifactBytes || ValidateAgeTool(ctx, tool, "age") != nil {
		return AgeStreamReceipt{}, errAgeStream
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	result := make(chan ageTarResult, 1)
	go func() {
		counter := &ageWriter{out: writer, maximum: r.PlainBytes + 65536, digest: sha256.New(), cancel: cancel}
		tw := tar.NewWriter(counter)
		manifest, _ := json.Marshal(ageManifest{1, r.Binding, r.PlainBytes})
		fail := func() { writer.CloseWithError(errAgeStream); cancel(); result <- ageTarResult{err: errAgeStream} }
		if tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest)), Typeflag: tar.TypeReg, Format: tar.FormatPAX}) != nil {
			fail()
			return
		}
		if _, err := tw.Write(manifest); err != nil {
			fail()
			return
		}
		if tw.WriteHeader(&tar.Header{Name: "payload", Mode: 0600, Size: r.PlainBytes, Typeflag: tar.TypeReg, Format: tar.FormatPAX}) != nil {
			fail()
			return
		}
		input := newAgeReader(child, source, r.PlainBytes)
		if _, err := io.CopyBuffer(tw, input, make([]byte, StreamBufferBytes)); err != nil || input.total != r.PlainBytes {
			fail()
			return
		}
		if tw.Close() != nil {
			fail()
			return
		}
		writer.Close()
		result <- ageTarResult{plain: input.result(), tarBytes: counter.total}
	}()
	cipher := &ageWriter{out: out, maximum: r.MaxCipherBytes, digest: sha256.New(), cancel: cancel}
	command := ageCommand(child, tool, "--encrypt", "--recipient", r.Recipient)
	command.Stdin = reader
	command.Stdout = cipher
	err := command.Run()
	reader.CloseWithError(errAgeStream)
	tarResult := <-result
	if err != nil || tarResult.err != nil || !validPackageContext(ctx) {
		return AgeStreamReceipt{}, errAgeStream
	}
	return AgeStreamReceipt{Status: "SEALED_STREAM_NOT_RESTORE_VERIFIED", Format: AgeBackupFormat, Binding: r.Binding, Plain: tarResult.plain, Cipher: cipher.result(), TarBytes: tarResult.tarBytes, ToolSHA256: tool.SHA256}, nil
}

func validateAgeIdentity(path string) error {
	if !intakeRoot(filepath.Dir(path)) || filepath.Base(path) == "." || intakeReparse(path) {
		return errAgeStream
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 16<<10 {
		return errAgeStream
	}
	root, file, err := openPackageFile(filepath.Dir(path), PackageArtifact{Name: filepath.Base(path), Bytes: info.Size()})
	if err != nil {
		return errAgeStream
	}
	defer root.Close()
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (16<<10)+1))
	if err != nil || len(raw) > 16<<10 || int64(len(raw)) != info.Size() {
		return errAgeStream
	}
	keys := 0
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !ageSecretIdentity.MatchString(line) {
			return errAgeStream
		}
		keys++
	}
	if keys != 1 {
		return errAgeStream
	}
	return nil
}

func sameAgeBinding(a, b Binding) bool {
	return a.RunID == b.RunID && a.Target == b.Target && a.Scope == b.Scope && a.CreatedAt.Equal(b.CreatedAt)
}

// VerifyAgeStream consumes decrypted bytes only into hashes. It never writes
// plaintext to caller storage; success requires the process/authentication EOF,
// exact encrypted size/hash, exact two tar entries and the encrypted binding.
func VerifyAgeStream(ctx context.Context, tool AgeTool, identity string, expected AgeStreamReceipt, ciphertext io.Reader, now time.Time) (AgeStreamReceipt, error) {
	return verifyAgeStreamTo(ctx, tool, identity, expected, ciphertext, now, nil)
}

// Internal-only sink: callers must authenticate before creating an exclusive
// protected plaintext file. VerifyAgeStream itself never exposes plaintext.
func verifyAgeStreamTo(ctx context.Context, tool AgeTool, identity string, expected AgeStreamReceipt, ciphertext io.Reader, now time.Time, sink io.Writer) (AgeStreamReceipt, error) {
	if ciphertext == nil || !validPackageContext(ctx) || expected.Format != AgeBackupFormat || !expected.Binding.valid(expected.Binding.Target, now) ||
		expected.AuthorizesExecution || expected.Plain.Bytes <= 0 || expected.Plain.Bytes > MaxStreamArtifactBytes-(1<<20) || expected.Cipher.Bytes <= 0 ||
		expected.Cipher.Bytes > MaxStreamArtifactBytes || expected.TarBytes < expected.Plain.Bytes || expected.TarBytes > expected.Plain.Bytes+65536 ||
		!fingerprint.MatchString(expected.Plain.SHA256) || !fingerprint.MatchString(expected.Cipher.SHA256) ||
		ValidateAgeTool(ctx, tool, "age") != nil || validateAgeIdentity(identity) != nil {
		return AgeStreamReceipt{}, errAgeStream
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	parsed := make(chan ageTarResult, 1)
	go func() {
		result := readAgeTarTo(child, reader, expected, sink)
		if result.err != nil {
			reader.CloseWithError(errAgeStream)
			cancel()
		}
		parsed <- result
	}()
	plain := &ageWriter{out: writer, maximum: expected.TarBytes, digest: sha256.New(), cancel: cancel}
	input := newAgeReader(child, ciphertext, expected.Cipher.Bytes)
	command := ageCommand(child, tool, "--decrypt", "--identity", identity)
	command.Stdin = input
	command.Stdout = plain
	err := command.Run()
	writer.CloseWithError(err)
	result := <-parsed
	if err != nil || result.err != nil || input.result() != expected.Cipher || result.plain != expected.Plain || plain.total != expected.TarBytes || !validPackageContext(ctx) {
		return AgeStreamReceipt{}, errAgeStream
	}
	expected.Status = "PASS_AUTHENTICATED_STREAM_ROUNDTRIP_ONLY"
	expected.ToolSHA256 = tool.SHA256
	return expected, nil
}

func readAgeTar(ctx context.Context, reader io.Reader, expected AgeStreamReceipt) ageTarResult {
	return readAgeTarTo(ctx, reader, expected, nil)
}

func readAgeTarTo(ctx context.Context, reader io.Reader, expected AgeStreamReceipt, sink io.Writer) ageTarResult {
	fail := func() ageTarResult { return ageTarResult{err: errAgeStream} }
	tr := tar.NewReader(reader)
	header, err := tr.Next()
	if err != nil || header.Name != "manifest.json" || header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > 4096 {
		return fail()
	}
	raw, err := io.ReadAll(io.LimitReader(tr, 4097))
	if err != nil || int64(len(raw)) != header.Size {
		return fail()
	}
	var manifest ageManifest
	if decodePackageJSON(bytes.NewReader(raw), 4096, &manifest) != nil || manifest.Version != 1 || manifest.PlainBytes != expected.Plain.Bytes || !sameAgeBinding(manifest.Binding, expected.Binding) {
		return fail()
	}
	header, err = tr.Next()
	if err != nil || header.Name != "payload" || header.Typeflag != tar.TypeReg || header.Size != expected.Plain.Bytes {
		return fail()
	}
	var payload io.Reader = tr
	if sink != nil {
		payload = io.TeeReader(tr, sink)
	}
	digest, err := DigestStream(ctx, payload, expected.Plain.Bytes, expected.Plain.Bytes)
	if err != nil || digest != expected.Plain {
		return fail()
	}
	if _, err = tr.Next(); err != io.EOF {
		return fail()
	}
	extra, err := io.ReadAll(io.LimitReader(reader, 1))
	if err != nil || len(extra) != 0 {
		return fail()
	}
	return ageTarResult{plain: digest}
}
