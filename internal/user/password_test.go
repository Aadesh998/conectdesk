package user

import "testing"

func TestGeneratePassword(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		p, err := GeneratePassword()
		if err != nil {
			t.Fatal(err)
		}
		if len(p) != 16 || !IsStrongPassword(p) {
			t.Fatalf("GeneratePassword() = %q, not a strong 16-character password", p)
		}
		if seen[p] {
			t.Fatalf("GeneratePassword() repeated %q", p)
		}
		seen[p] = true
	}
}
