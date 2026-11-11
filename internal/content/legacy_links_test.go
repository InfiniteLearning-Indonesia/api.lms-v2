package content

import (
	"errors"
	"testing"
)

func TestLegacyImportantLinksPreview(t *testing.T) {
	live, err := PreviewLegacyImportantLinks("live-v1", []byte(`[{"id":"one","title":"Learning","url":"https://example.org/learning","iconType":"book"}]`))
	if err != nil || len(live) != 1 || live[0].Disposition != "REVIEW_REQUIRED" || live[0].SourceScope != "" {
		t.Fatalf("live preview must retain unknown scope: %v %#v", err, live)
	}
	staging, err := PreviewLegacyImportantLinks("staging-v2", []byte(`[{"id":"two","title":"Program guide","url":"https://example.org/guide","scope":"restricted","programs":["Program A"]}]`))
	if err != nil || len(staging) != 1 || staging[0].SourceScope != "restricted" || staging[0].SourcePrograms[0] != "Program A" || staging[0].Disposition != "MAPPED_CANDIDATE_REVIEW" {
		t.Fatalf("staging scope should remain a candidate: %v %#v", err, staging)
	}
	for _, tc := range []struct{ version, payload string }{
		{"unknown-v3", `[]`},
		{"live-v1", `[{"id":"x","title":"Bad","url":"javascript:alert(1)"}]`},
		{"live-v1", `[{"id":"x","title":"Bad","url":"https://example.org","scope":"mandatory"}]`},
		{"staging-v2", `[{"id":"x","title":"Bad","url":"https://example.org","scope":"restricted"}]`},
		{"staging-v2", `[{"id":"x","title":"Bad","url":"https://example.org","scope":"mandatory","programs":["Program A"]}]`},
		{"live-v1", `[{"id":"x","title":"Bad","url":"https://localhost/private"}]`},
		{"live-v1", `[{"id":"x","title":"Bad","url":"https://example.org","unknown":true}]`},
		{"live-v1", `[{"id":"x","title":"Bad","url":"https://example.org"}] {}`},
		{"live-v1", `[{"id":"x","title":"Bad","url":"https://example.org"},{"id":"x","title":"Again","url":"https://example.org"}]`},
	} {
		if _, err := PreviewLegacyImportantLinks(tc.version, []byte(tc.payload)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid legacy payload accepted: %s %s (%v)", tc.version, tc.payload, err)
		}
	}
}
