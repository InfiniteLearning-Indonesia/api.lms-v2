package backup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const MaxPackageJSON = 256 << 10
const MaxInventoryJSON = 2 << 20

var errPackage = errors.New("BASELINE_PACKAGE_REJECTED")

// decodePackageJSON applies exact field casing recursively, including objects
// inside arrays. No decoder detail or input value is exposed to callers.
func decodePackageJSON(r io.Reader, maximum int64, destination any) error {
	raw, err := io.ReadAll(io.LimitReader(r, maximum+1))
	if err != nil || int64(len(raw)) > maximum || !utf8.Valid(raw) {
		return errPackage
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if scanIntakeJSON(d, 0) != nil {
		return errPackage
	}
	if _, err := d.Token(); err != io.EOF {
		return errPackage
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(destination) != nil {
		return errPackage
	}
	canonical, err := json.Marshal(destination)
	if err != nil || !exactPackageKeys(raw, canonical) {
		return errPackage
	}
	return nil
}

func exactPackageKeys(raw, canonical []byte) bool {
	raw, canonical = bytes.TrimSpace(raw), bytes.TrimSpace(canonical)
	if len(raw) == 0 || len(canonical) == 0 {
		return false
	}
	switch raw[0] {
	case '{':
		var a, b map[string]json.RawMessage
		if json.Unmarshal(raw, &a) != nil || json.Unmarshal(canonical, &b) != nil || a == nil || b == nil {
			return false
		}
		for key, value := range a {
			other, ok := b[key]
			if !ok || !exactPackageKeys(value, other) {
				return false
			}
		}
	case '[':
		var a, b []json.RawMessage
		if json.Unmarshal(raw, &a) != nil || json.Unmarshal(canonical, &b) != nil || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !exactPackageKeys(a[i], b[i]) {
				return false
			}
		}
	}
	return true
}

func DecodePackage(r io.Reader) (PackageRequest, error) {
	var request PackageRequest
	if decodePackageJSON(r, MaxPackageJSON, &request) != nil || request.Version != 1 {
		return PackageRequest{}, errPackage
	}
	return request, nil
}

func DecodeRestorePlan(r io.Reader) (RestorePlanRequest, error) {
	var request RestorePlanRequest
	if decodePackageJSON(r, MaxPackageJSON, &request) != nil || request.Version != 1 {
		return RestorePlanRequest{}, errPackage
	}
	return request, nil
}
