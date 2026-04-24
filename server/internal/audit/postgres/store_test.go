package auditpg

import "testing"

func TestCloneMapCopiesValues(t *testing.T) {
	src := map[string]any{"action": "create"}
	dst := cloneMap(src)
	src["action"] = "update"

	if dst["action"] != "create" {
		t.Fatalf("expected copied value, got %v", dst["action"])
	}
}
