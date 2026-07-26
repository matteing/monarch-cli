package command

import "testing"

func TestValidateTransactionGroup(t *testing.T) {
	for _, group := range []string{"month", "none"} {
		if err := validateTransactionGroup(group); err != nil {
			t.Fatalf("group %q rejected: %v", group, err)
		}
	}
	if err := validateTransactionGroup("week"); err == nil {
		t.Fatal("invalid group accepted")
	}
}
