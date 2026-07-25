package catalog

import "testing"

func TestCatalogContainsSixBuiltInModels(t *testing.T) {
	models := All()
	if len(models) != 6 {
		t.Fatalf("All returned %d models, want 6", len(models))
	}
	for _, model := range models {
		found, ok := Find(model.Descriptor().ID)
		if !ok || found.Descriptor().ID != model.Descriptor().ID {
			t.Errorf("Find failed for %q", model.Descriptor().ID)
		}
	}
	if _, ok := Find("unknown"); ok {
		t.Fatal("Find returned an unknown model")
	}
}
