package migrations

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/conectdesk/conectdesk/internal/testutil"
)

func float32Bytes(vec ...float32) []byte {
	b := make([]byte, 4*len(vec))
	for i, f := range vec {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// TestV2_37_0 puts the old BYTEA embeddings table back, upgrades it, and checks every stored
// vector survives as a pgvector value without re-embedding, unusable rows are dropped with their
// source's fingerprint cleared, and the migration can run twice.
func TestV2_37_0(t *testing.T) {
	db := testutil.NewDB(t, "migration_pgvector_embeddings")
	db.MustExec(`ALTER TABLE embeddings DROP COLUMN embedding`)
	db.MustExec(`ALTER TABLE embeddings ADD COLUMN embedding BYTEA`)
	db.MustExec(`INSERT INTO ai_knowledge_base (tenant_id, id, title, content, embedded_fingerprint) VALUES (1, 1, 'Fees', 'fees', 'fp1'), (1, 2, 'Broken', 'x', 'fp2')`)
	db.MustExec(`INSERT INTO embeddings (tenant_id, source_type, source_id, chunk_text, embedding, dimensions) VALUES
		(1, 'snippet', 1, 'fees', $1, 3), (1, 'snippet', 2, 'broken', NULL, 0), (1, 'tag', 7, 'billing', $2, 3)`,
		float32Bytes(1, -0.5, 0.25), float32Bytes(0, 1, 0))

	for i := range 2 {
		if err := V2_37_0(db, nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var got []struct {
		SourceID   int    `db:"source_id"`
		Vec        string `db:"vec"`
		Dimensions int    `db:"dimensions"`
	}
	if err := db.Select(&got, `SELECT source_id, embedding::text AS vec, dimensions FROM embeddings ORDER BY source_id`); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Vec != "[1,-0.5,0.25]" || got[0].Dimensions != 3 || got[1].Vec != "[0,1,0]" {
		t.Fatalf("embeddings = %+v", got)
	}

	var fp1, fp2 string
	db.Get(&fp1, `SELECT embedded_fingerprint FROM ai_knowledge_base WHERE id = 1`)
	db.Get(&fp2, `SELECT embedded_fingerprint FROM ai_knowledge_base WHERE id = 2`)
	if fp1 != "fp1" || fp2 != "" {
		t.Fatalf("fingerprints = %q, %q; want the converted one kept and the broken one cleared", fp1, fp2)
	}

	var score float64
	if err := db.Get(&score, `SELECT 1 - (embedding <=> '[1,-0.5,0.25]') FROM embeddings WHERE source_id = 1`); err != nil || score < 0.999 {
		t.Fatalf("cosine search on converted vector = %v, %v", score, err)
	}
}
