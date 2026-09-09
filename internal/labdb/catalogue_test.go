package labdb

import (
	"testing"

	"lmsv3/internal/migration/runner"
)

func TestCatalogueIsForwardOnly(t *testing.T) {
	if err := runner.ValidateCatalogue(Catalogue()); err != nil {
		t.Fatal(err)
	}
	if Catalogue()[0].Checksum() != Checksum() {
		t.Fatal("historical base checksum changed")
	}
}
