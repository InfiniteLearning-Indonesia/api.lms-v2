//go:build qaage

package backup

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func actualAgeFixture(t *testing.T) (context.Context, AgeTool, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	tool := AgeTool{os.Getenv("QA_AGE_BINARY"), os.Getenv("QA_AGE_SHA256"), os.Getenv("QA_AGE_MODULE_SUM")}
	keygen := AgeTool{os.Getenv("QA_AGE_KEYGEN_BINARY"), os.Getenv("QA_AGE_KEYGEN_SHA256"), tool.ModuleSum}
	if ValidateAgeTool(ctx, tool, "age") != nil || ValidateAgeTool(ctx, keygen, "age-keygen") != nil {
		t.Fatal("VERIFIED_AGE_TOOLS_REQUIRED; not skipped")
	}
	command := ageCommand(ctx, keygen)
	var raw bytes.Buffer
	command.Stdout = &raw
	if command.Run() != nil || raw.Len() > 4096 {
		t.Fatal("fixture key generation failed")
	}
	recipient := ""
	for _, line := range strings.Split(raw.String(), "\n") {
		if strings.HasPrefix(line, "# public key: ") {
			recipient = strings.TrimPrefix(line, "# public key: ")
		}
	}
	if !ageRecipient.MatchString(recipient) {
		t.Fatal("fixture recipient missing")
	}
	path := filepath.Join(t.TempDir(), "identity.txt")
	if os.WriteFile(path, raw.Bytes(), 0600) != nil {
		t.Fatal("fixture key unavailable")
	}
	return ctx, tool, path, recipient
}

func TestAgeActualLargeRoundTrip(t *testing.T) {
	ctx, tool, key, recipient := actualAgeFixture(t)
	binding := ageFixtureReceipt().Binding
	file, err := os.OpenFile(filepath.Join(t.TempDir(), "payload.age"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal("fixture output unavailable")
	}
	defer file.Close()
	const size = (33 << 20) + 17
	r, err := SealAgeStream(ctx, tool, AgeStreamRequest{binding, recipient, size, size + (1 << 20)}, io.LimitReader(&syntheticStream{}, size), file, time.Now().UTC())
	if err != nil {
		t.Fatal("large age encryption failed")
	}
	if _, err = file.Seek(0, 0); err != nil {
		t.Fatal("fixture seek")
	}
	verified, err := VerifyAgeStream(ctx, tool, key, r, file, time.Now().UTC())
	if err != nil || verified.Status != "PASS_AUTHENTICATED_STREAM_ROUNDTRIP_ONLY" || verified.AuthorizesExecution || verified.Plain.Bytes != size {
		t.Fatal("large authenticated roundtrip failed")
	}
}

func TestAgeActualTamperAndWrongKey(t *testing.T) {
	ctx, tool, key, recipient := actualAgeFixture(t)
	var cipher bytes.Buffer
	r, err := SealAgeStream(ctx, tool, AgeStreamRequest{ageFixtureReceipt().Binding, recipient, 4, 65540}, strings.NewReader("data"), &cipher, time.Now().UTC())
	if err != nil {
		t.Fatal("fixture seal failed")
	}
	_, _, otherKey, _ := actualAgeFixture(t)
	if _, err := VerifyAgeStream(ctx, tool, otherKey, r, bytes.NewReader(cipher.Bytes()), time.Now().UTC()); err == nil {
		t.Fatal("wrong key accepted")
	}
	for _, data := range [][]byte{append([]byte(nil), cipher.Bytes()[:cipher.Len()-1]...), append(append([]byte(nil), cipher.Bytes()...), 'x')} {
		bad := r
		bad.Cipher = StreamDigest{int64(len(data)), Hash(data)}
		if _, err := VerifyAgeStream(ctx, tool, key, bad, bytes.NewReader(data), time.Now().UTC()); err == nil {
			t.Fatal("truncation/trailing ciphertext accepted")
		}
	}
	changed := append([]byte(nil), cipher.Bytes()...)
	changed[len(changed)-1] ^= 1
	bad := r
	bad.Cipher = StreamDigest{int64(len(changed)), Hash(changed)}
	if _, err := VerifyAgeStream(ctx, tool, key, bad, bytes.NewReader(changed), time.Now().UTC()); err == nil {
		t.Fatal("forged auth tag accepted even with updated outer digest")
	}
}

func TestAgeActualBindingAndShortInput(t *testing.T) {
	ctx, tool, key, recipient := actualAgeFixture(t)
	var cipher bytes.Buffer
	r, err := SealAgeStream(ctx, tool, AgeStreamRequest{ageFixtureReceipt().Binding, recipient, 4, 65540}, strings.NewReader("data"), &cipher, time.Now().UTC())
	if err != nil {
		t.Fatal("fixture seal failed")
	}
	r.Binding.RunID = "different"
	if _, err := VerifyAgeStream(ctx, tool, key, r, bytes.NewReader(cipher.Bytes()), time.Now().UTC()); err == nil {
		t.Fatal("changed binding accepted")
	}
	for _, input := range []string{"abc", "abcde"} {
		var out bytes.Buffer
		if value, err := SealAgeStream(ctx, tool, AgeStreamRequest{ageFixtureReceipt().Binding, recipient, 4, 65540}, strings.NewReader(input), &out, time.Now().UTC()); err == nil || value.Status != "" {
			t.Fatal("short/trailing input produced receipt")
		}
	}
}

func TestAgeActualCancellationAndWriterFailure(t *testing.T) {
	ctx, tool, _, recipient := actualAgeFixture(t)
	var out bytes.Buffer
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := SealAgeStream(stopped, tool, AgeStreamRequest{ageFixtureReceipt().Binding, recipient, 4, 65540}, strings.NewReader("data"), &out, time.Now().UTC()); err == nil || out.Len() != 0 {
		t.Fatal("canceled operation wrote data")
	}
	if value, err := SealAgeStream(ctx, tool, AgeStreamRequest{ageFixtureReceipt().Binding, recipient, 4, 65540}, strings.NewReader("data"), failAgeWriter{}, time.Now().UTC()); err == nil || value.Status != "" {
		t.Fatal("failed writer produced receipt")
	}
}

type failAgeWriter struct{}

func (failAgeWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
