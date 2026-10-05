package ai

import (
	"context"
	"testing"

	"github.com/conectdesk/conectdesk/internal/ai/models"
	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/testutil"
)

func TestVectorLiteral(t *testing.T) {
	if got, want := vectorLiteral([]float32{1, -0.25, 3.5e-7}), "[1,-0.25,3.5e-07]"; got != want {
		t.Fatalf("vectorLiteral = %q, want %q", got, want)
	}
	if got := vectorLiteral(nil); got != "[]" {
		t.Fatalf("vectorLiteral(nil) = %q, want []", got)
	}
}

// TestSearchVectors stores vectors in pgvector and checks search ranks by cosine similarity,
// filters by source type, skips vectors of another length and keeps tenants apart.
func TestSearchVectors(t *testing.T) {
	db := testutil.NewDB(t, "ai_vector_search")
	db.MustExec(`INSERT INTO tenants (id, slug, "name") VALUES (2, 'acme', 'Acme'), (3, 'globex', 'Globex')`)
	m := newTestManager(t)
	m.db = db
	if err := dbutil.ScanSQLFile("queries.sql", &m.q, db, efs); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	commit := func(tenantID int, sourceType string, ids []int, chunks []indexedChunk) {
		t.Helper()
		if err := m.commitEmbeddings(tenantID, sourceType, ids, chunks); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
	commit(2, models.SourceSnippet, []int{1, 2, 3}, []indexedChunk{
		{sourceType: models.SourceSnippet, sourceID: 1, chunkText: "fees", vec: []float32{1, 0, 0}},
		{sourceType: models.SourceSnippet, sourceID: 2, chunkText: "timings", vec: []float32{0.6, 0.8, 0}},
		{sourceType: models.SourceSnippet, sourceID: 3, chunkText: "old model", vec: []float32{1, 0}},
	})
	commit(2, models.SourceTag, []int{1}, []indexedChunk{{sourceType: models.SourceTag, sourceID: 1, chunkText: "billing", vec: []float32{1, 0, 0}}})
	commit(3, models.SourceSnippet, []int{1}, []indexedChunk{{sourceType: models.SourceSnippet, sourceID: 1, chunkText: "globex secret", vec: []float32{1, 0, 0}}})

	results, err := m.searchVectors(2, ctx, []float32{1, 0, 0}, 10, models.SourceSnippet, models.SourceHelpArticle)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].ChunkText != "fees" || results[1].ChunkText != "timings" {
		t.Fatalf("results = %+v, want fees then timings", results)
	}
	if results[0].Score < 0.999 || results[1].Score < 0.59 || results[1].Score > 0.61 {
		t.Fatalf("scores = %v, %v, want 1 and 0.6", results[0].Score, results[1].Score)
	}

	if results, _ := m.searchVectors(2, ctx, []float32{1, 0, 0}, 1, models.SourceSnippet); len(results) != 1 {
		t.Fatalf("k=1 returned %d results", len(results))
	}
	if results, _ := m.searchVectors(2, ctx, []float32{1, 0, 0}, 10, models.SourceTag); len(results) != 1 || results[0].ChunkText != "billing" {
		t.Fatalf("tag search = %+v, want only billing", results)
	}
	if results, _ := m.searchVectors(4, ctx, []float32{1, 0, 0}, 10, models.SourceSnippet); len(results) != 0 {
		t.Fatalf("tenant without vectors found %+v", results)
	}

	// Re-committing a source replaces its chunks; removing it drops them.
	commit(2, models.SourceSnippet, []int{2}, []indexedChunk{{sourceType: models.SourceSnippet, sourceID: 2, chunkText: "new timings", vec: []float32{0, 1, 0}}})
	if err := m.removeEmbeddings(2, models.SourceSnippet, 1); err != nil {
		t.Fatal(err)
	}
	results, _ = m.searchVectors(2, ctx, []float32{1, 0, 0}, 10, models.SourceSnippet)
	if len(results) != 1 || results[0].ChunkText != "new timings" {
		t.Fatalf("after replace and remove = %+v, want only new timings", results)
	}
	if results, _ := m.searchVectors(3, ctx, []float32{1, 0, 0}, 10, models.SourceSnippet); len(results) != 1 || results[0].ChunkText != "globex secret" {
		t.Fatalf("acme's writes touched globex: %+v", results)
	}

	texts, err := m.getTagEmbeddingTexts(2)
	if err != nil || texts[1] != "billing" || len(texts) != 1 {
		t.Fatalf("tag texts = %v, %v", texts, err)
	}
}
