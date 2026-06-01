package atlas_test

import (
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas/storetest"
)

// TestMemStore_Contract runs the shared Store contract suite against
// MemStore. The suite is implementation-agnostic, so a future
// downstream-backed Store can reuse it via the same factory shape. Any
// failure here is a divergence between the docstring contract on
// atlas.Store and what MemStore actually does.
func TestMemStore_Contract(t *testing.T) {
	storetest.RunContractSuite(t, storetest.MemStoreFactory)
}
