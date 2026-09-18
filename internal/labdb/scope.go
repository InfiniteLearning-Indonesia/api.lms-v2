package labdb

import (
	"errors"

	"lmsv3/internal/migration/runner"
)

// SyntheticCatalogueScope exists only for historical fresh-cluster QA journeys.
// Runtime readiness stays exact for the explicitly selected fixed prefix.
func SyntheticCatalogueScope(scope string) ([]runner.Migration, error) {
	all := Catalogue()
	lengths := map[string]int{
		"":             len(all),
		"pre-m06":      6,
		"pre-m07":      8,
		"pre-m07-cp02": 9,
		"pre-m07-cp03": 10,
		"pre-m08-cp01": 11,
		"pre-m08-cp02": 12,
		"pre-m08-cp03": 13,
		"pre-m08-cp04": 14,
		"pre-m11-cp01": 15,
		"pre-m12-cp01": 16,
		"pre-m12-cp02": 17,
		"pre-m12-cp03": 18,
		"pre-m13-cp01": 19,
		"pre-m13-cp02": 20,
		"pre-m09-cp01": 21,
		"pre-m09-cp02": 22,
	}
	n, ok := lengths[scope]
	if !ok || n < 1 || n > len(all) {
		return nil, errors.New("SYNTHETIC_CATALOGUE_SCOPE_REJECTED")
	}
	return all[:n], nil
}
