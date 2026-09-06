package baseline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFingerprintDoesNotEmitCredentials(t *testing.T) {
	a := ParseConnectionFingerprint("synthetic", []byte("DB_HOST=synthetic.invalid\nDB_PORT=5432\nDB_USER=reader\nDB_NAME=lms\nDB_PASSWORD=DO_NOT_EMIT\nAPI_KEY=DO_NOT_EMIT"))
	b := ParseConnectionFingerprint("other", []byte("DB_HOST=synthetic.invalid\nDB_PORT=5432\nDB_USER=reader\nDB_NAME=lms\nDB_PASSWORD=different"))
	if !a.Complete || a.Fingerprint != b.Fingerprint || a.VerifiedDatabase {
		t.Fatal("configuration comparison must not assert identity")
	}
	data, _ := json.Marshal(a)
	for _, secret := range []string{"DO_NOT_EMIT", "synthetic.invalid", "reader"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("raw configuration exposed")
		}
	}
}

func TestFingerprintFailsClosedOnAmbiguity(t *testing.T) {
	base := "DB_HOST=host.invalid\nDB_PORT=5432\nDB_USER=reader\nDB_NAME=lms\n"
	for _, raw := range []string{"", base + "DB_HOST=other", base + "DB_NAME=${NAME}", base + "DB_USERNAME=other", base + "DB_DATABASE=other", strings.Replace(base, "DB_HOST=host.invalid", "DB_HOST='unterminated", 1)} {
		if ParseConnectionFingerprint("test", []byte(raw)).Complete {
			t.Fatalf("ambiguous config accepted")
		}
	}
}

func TestDeclarationsAreSanitizedAndHaveLocations(t *testing.T) {
	input := "@Entity('users')\n@Controller('users')\n@Post(\n  'invite'\n)\nconst secret = 'DO_NOT_EMIT'; setInterval(job, 1000);\n// @Delete()"
	got := ScanDeclarations("src/example.ts", input)
	if len(got) != 4 {
		t.Fatalf("got %d declarations", len(got))
	}
	if got[2].Name != "POST" || got[2].Line != 3 {
		t.Fatal("multiline decorator start lost")
	}
	data, _ := json.Marshal(got)
	if strings.Contains(string(data), "DO_NOT_EMIT") {
		t.Fatal("source literal exposed")
	}
}
