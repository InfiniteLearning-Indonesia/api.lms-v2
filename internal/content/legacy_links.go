package content

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"strings"
	"unicode"
)

// LegacyLinkPreview is an operator-side candidate, never an automatically
// published Class resource or permission. The source JSON remains with the
// separately governed migration inventory.
type LegacyLinkPreview struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	URL            string   `json:"url"`
	IconType       string   `json:"icon_type,omitempty"`
	SourceScope    string   `json:"source_scope,omitempty"`
	SourcePrograms []string `json:"source_programs,omitempty"`
	Disposition    string   `json:"disposition"`
}

type legacyLinkV1 struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	IconType string `json:"iconType,omitempty"`
}

type legacyLinkV2 struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	IconType string   `json:"iconType,omitempty"`
	Scope    string   `json:"scope,omitempty"`
	Programs []string `json:"programs,omitempty"`
}

func decodeLegacyLinks(raw []byte, target any) error {
	if len(raw) < 2 || len(raw) > 65536 || raw[0] != '[' {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrInvalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	return nil
}

func validLegacyLink(id, title, rawURL, icon string) bool {
	if len(id) < 1 || len(id) > 80 || len(icon) > 80 || strings.TrimSpace(id) != id || strings.TrimSpace(icon) != icon {
		return false
	}
	for _, value := range []string{id, icon} {
		for _, character := range value {
			if unicode.IsControl(character) {
				return false
			}
		}
	}
	if _, err := cleanTitle(title); err != nil || strings.TrimSpace(title) != title || len(rawURL) < 9 || len(rawURL) > 2048 || strings.TrimSpace(rawURL) != rawURL {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" {
		return false
	}
	host := parsed.Hostname()
	return strings.Contains(host, ".") && net.ParseIP(host) == nil && !strings.HasSuffix(strings.ToLower(host), ".local")
}

// PreviewLegacyImportantLinks validates the two shapes observed in the live
// and staging codebases. It performs no network request and no DB mutation.
// Even a valid preview still requires a reviewed Class/source mapping.
func PreviewLegacyImportantLinks(version string, raw []byte) ([]LegacyLinkPreview, error) {
	result := []LegacyLinkPreview{}
	seen := map[string]bool{}
	appendLink := func(id, title, linkURL, icon, scope string, programs []string) error {
		if !validLegacyLink(id, title, linkURL, icon) || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
		disposition := "REVIEW_REQUIRED"
		if scope != "" {
			if scope != "mandatory" && scope != "restricted" && scope != "personal" {
				return ErrInvalid
			}
			if (scope == "restricted") != (len(programs) > 0) {
				return ErrInvalid
			}
			for _, program := range programs {
				if _, err := cleanTitle(program); err != nil || strings.TrimSpace(program) != program {
					return ErrInvalid
				}
			}
			disposition = "MAPPED_CANDIDATE_REVIEW"
		} else if len(programs) > 0 {
			return ErrInvalid
		}
		result = append(result, LegacyLinkPreview{ID: id, Title: title, URL: linkURL, IconType: icon,
			SourceScope: scope, SourcePrograms: programs, Disposition: disposition})
		return nil
	}
	switch version {
	case "live-v1":
		var links []legacyLinkV1
		if err := decodeLegacyLinks(raw, &links); err != nil || len(links) > 100 {
			return nil, ErrInvalid
		}
		for _, link := range links {
			if err := appendLink(link.ID, link.Title, link.URL, link.IconType, "", nil); err != nil {
				return nil, err
			}
		}
	case "staging-v2":
		var links []legacyLinkV2
		if err := decodeLegacyLinks(raw, &links); err != nil || len(links) > 100 {
			return nil, ErrInvalid
		}
		for _, link := range links {
			if err := appendLink(link.ID, link.Title, link.URL, link.IconType, link.Scope, link.Programs); err != nil {
				return nil, err
			}
		}
	default:
		return nil, ErrInvalid
	}
	return result, nil
}
