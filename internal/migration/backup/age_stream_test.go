package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func ageFixtureReceipt() AgeStreamReceipt {
	return AgeStreamReceipt{Format: AgeBackupFormat, Binding: Binding{RunID: "fixture", Target: strings.Repeat("a", 64), Scope: strings.Repeat("b", 64), CreatedAt: time.Now().UTC()},
		Plain: StreamDigest{4, Hash([]byte("data"))}, Cipher: StreamDigest{100, strings.Repeat("c", 64)}, TarBytes: 3072}
}

func ageFixtureTar(t *testing.T, r AgeStreamReceipt, extra bool) []byte {
	t.Helper()
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	raw, _ := json.Marshal(ageManifest{1, r.Binding, r.Plain.Bytes})
	for _, part := range []struct {
		name string
		data []byte
	}{{"manifest.json", raw}, {"payload", []byte("data")}} {
		if tw.WriteHeader(&tar.Header{Name: part.name, Mode: 0600, Size: int64(len(part.data)), Typeflag: tar.TypeReg}) != nil {
			t.Fatal("fixture header")
		}
		if _, err := tw.Write(part.data); err != nil {
			t.Fatal("fixture payload")
		}
	}
	if extra {
		tw.WriteHeader(&tar.Header{Name: "extra", Mode: 0600, Size: 1, Typeflag: tar.TypeReg})
		tw.Write([]byte("x"))
	}
	if tw.Close() != nil {
		t.Fatal("fixture close")
	}
	return out.Bytes()
}

func TestAgeToolRefusal(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	dir := t.TempDir()
	name := "age"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	raw := []byte("synthetic nonexecutable, never run")
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("fixture write")
	}
	tool := AgeTool{path, Hash(raw), "h1:" + strings.Repeat("A", 43) + "="}
	if ValidateAgeTool(ctx, tool, "age") == nil {
		t.Fatal("non-Go or unpinned binary accepted")
	}
	tool.SHA256 = strings.Repeat("f", 64)
	if ValidateAgeTool(ctx, tool, "age") == nil || ValidateAgeTool(context.Background(), tool, "age") == nil {
		t.Fatal("checksum/deadline bypass")
	}
	tool.Path = filepath.Join(dir, "other.exe")
	if ValidateAgeTool(ctx, tool, "age") == nil || ValidateAgeTool(ctx, tool, "shell") == nil {
		t.Fatal("arbitrary executable accepted")
	}
}

func TestAgeEnvelopeBindingAndFraming(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	r := ageFixtureReceipt()
	raw := ageFixtureTar(t, r, false)
	if result := readAgeTar(ctx, bytes.NewReader(raw), r); result.err != nil || result.plain != r.Plain {
		t.Fatal("standard tar fixture rejected")
	}
	wrong := r
	wrong.Binding.RunID = "different"
	if readAgeTar(ctx, bytes.NewReader(raw), wrong).err == nil {
		t.Fatal("binding not protected by envelope layout")
	}
	wrong = r
	wrong.Plain.SHA256 = strings.Repeat("f", 64)
	for _, bad := range [][]byte{raw[:1024], append(append([]byte(nil), raw...), 'x'), ageFixtureTar(t, r, true)} {
		if readAgeTar(ctx, bytes.NewReader(bad), r).err == nil {
			t.Fatal("truncated/trailing/extra tar accepted")
		}
	}
	if readAgeTar(ctx, bytes.NewReader(raw), wrong).err == nil {
		t.Fatal("payload digest mismatch accepted")
	}
	// This is only tar/binding validation, not proof of age encryption.
}

func TestAgeProcessEnvironmentAndIdentityBoundary(t *testing.T) {
	t.Setenv("DATABASE_URL", "PRIVATE_FIXTURE_SECRET")
	t.Setenv("AGE_PLUGIN_PATH", "PRIVATE_FIXTURE_SECRET")
	t.Setenv("PATH", "PRIVATE_FIXTURE_SECRET")
	if strings.Contains(strings.Join(ageEnvironment(), "\n"), "PRIVATE_FIXTURE_SECRET") {
		t.Fatal("credential/plugin search environment inherited")
	}
	dir := t.TempDir()
	for i, content := range []string{"AGE-PLUGIN-TEST-1malicious\n", "age-encryption.org/v1\n", "AGE-SECRET-KEY-1" + strings.Repeat("A", 60) + "\nAGE-SECRET-KEY-1" + strings.Repeat("A", 60), strings.Repeat("x", 17000)} {
		path := filepath.Join(dir, string(rune('a'+i))+".txt")
		os.WriteFile(path, []byte(content), 0600)
		if validateAgeIdentity(path) == nil {
			t.Fatal("plugin/encrypted/multiple/oversized key fixture accepted")
		}
	}
}

func TestAgeStreamRefusesMissingBudgetAndTool(t *testing.T) {
	ctx, cancel := packageContext()
	defer cancel()
	receipt := ageFixtureReceipt()
	request := AgeStreamRequest{Binding: receipt.Binding, Recipient: "age1" + strings.Repeat("a", 58), PlainBytes: 4, MaxCipherBytes: 65540}
	var out bytes.Buffer
	never := streamReaderFunc(func([]byte) (int, error) { t.Fatal("invalid tool reached input"); return 0, io.EOF })
	if value, err := SealAgeStream(ctx, AgeTool{}, request, never, &out, time.Now().UTC()); err == nil || value.Status != "" || out.Len() != 0 {
		t.Fatal("missing tool promoted ciphertext")
	}
	if value, err := VerifyAgeStream(ctx, AgeTool{}, "missing", receipt, never, time.Now().UTC()); err == nil || value.Status != "" {
		t.Fatal("missing tool promoted plaintext")
	}
	request.Recipient = "--plugin-command"
	if _, err := SealAgeStream(ctx, AgeTool{}, request, never, &out, time.Now().UTC()); err == nil {
		t.Fatal("arbitrary flags accepted")
	}
}
