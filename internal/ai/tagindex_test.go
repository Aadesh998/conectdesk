package ai

import (
	"reflect"
	"testing"

	"github.com/conectdesk/conectdesk/internal/ai/models"
)

func TestDiffTagIndex(t *testing.T) {
	indexed := map[int]string{1: "billing", 2: "old-name", 3: "deleted", 4: "blanked"}
	tags := []models.TagRef{
		{ID: 1, Name: "billing"},
		{ID: 2, Name: "new-name"},
		{ID: 4, Name: "  "},
		{ID: 6, Name: "kyc"},
	}

	stale, removed := diffTagIndex(tags, indexed)

	wantStale := []models.TagRef{{ID: 2, Name: "new-name"}, {ID: 6, Name: "kyc"}}
	if !reflect.DeepEqual(stale, wantStale) {
		t.Errorf("stale = %v, want %v", stale, wantStale)
	}
	if want := []int{3, 4}; !reflect.DeepEqual(removed, want) {
		t.Errorf("removed = %v, want %v", removed, want)
	}
}
