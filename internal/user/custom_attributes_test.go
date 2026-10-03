package user

import "testing"

// TestFillEmptyCustomAttributes checks only missing or empty attributes are
// set, and a value already there is kept.
func TestFillEmptyCustomAttributes(t *testing.T) {
	u, db := newTestManager(t)
	var id int
	db.QueryRow(`INSERT INTO users (tenant_id, type, first_name, custom_attributes)
		VALUES (1, 'contact', 'Sunita', '{"student_name": "Kabir", "city": "", "class": null}') RETURNING id`).Scan(&id)

	got, err := u.GetCustomAttributes(testTenant, id)
	if err != nil || got["student_name"] != "Kabir" {
		t.Fatalf("GetCustomAttributes = %v, %v", got, err)
	}
	merged, err := u.FillEmptyCustomAttributes(testTenant, id, map[string]any{
		"student_name": "Sunita ji", "city": "Ahmedabad", "class": "Class 12", "course": "NEET",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"student_name": "Kabir", "city": "Ahmedabad", "class": "Class 12", "course": "NEET"}
	for k, v := range want {
		if merged[k] != v {
			t.Errorf("%s = %v, want %v", k, merged[k], v)
		}
	}
}
