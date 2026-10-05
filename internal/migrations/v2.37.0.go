package migrations

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/stuffbin"
	"github.com/lib/pq"
)

// v2_37_0Batch is how many stored vectors are converted per round trip.
const v2_37_0Batch = 500

// V2_37_0 moves embeddings from BYTEA blobs searched in memory to pgvector, searched in the
// database. Stored vectors are converted in place, so nothing is re-embedded with the provider.
func V2_37_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf) error {
	if _, err := db.Exec(`CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		return fmt.Errorf("enabling the pgvector extension (install pgvector on the Postgres server, e.g. the pgvector/pgvector:pg17 image): %w", err)
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var dataType string
	if err := tx.Get(&dataType, `SELECT data_type FROM information_schema.columns WHERE table_name = 'embeddings' AND column_name = 'embedding'`); err != nil {
		return fmt.Errorf("reading embeddings.embedding type: %w", err)
	}
	if dataType == "bytea" {
		if err := convertEmbeddingsToVector(tx); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS index_embeddings_on_tenant_source_type_dimensions ON embeddings (tenant_id, source_type, dimensions)`); err != nil {
		return err
	}
	return tx.Commit()
}

func convertEmbeddingsToVector(tx *sqlx.Tx) error {
	// Rows without a usable vector were never searchable. Drop them and clear their source's
	// fingerprint so the reconciler embeds just those again; tags are re-embedded when missing.
	var broken []struct {
		TenantID   int    `db:"tenant_id"`
		SourceType string `db:"source_type"`
		SourceID   int64  `db:"source_id"`
	}
	if err := tx.Select(&broken, `
		DELETE FROM embeddings
		WHERE embedding IS NULL OR length(embedding) = 0 OR length(embedding) % 4 <> 0
		RETURNING tenant_id, source_type, source_id`); err != nil {
		return fmt.Errorf("dropping unusable embeddings: %w", err)
	}
	for _, b := range broken {
		var q string
		switch b.SourceType {
		case "snippet":
			q = `UPDATE ai_knowledge_base SET embedded_fingerprint = '' WHERE tenant_id = $1 AND id = $2`
		case "help_article":
			q = `UPDATE help_articles SET embedded_fingerprint = '' WHERE tenant_id = $1 AND id = $2`
		default:
			continue
		}
		if _, err := tx.Exec(q, b.TenantID, b.SourceID); err != nil {
			return fmt.Errorf("clearing fingerprint of %s %d: %w", b.SourceType, b.SourceID, err)
		}
	}

	if _, err := tx.Exec(`ALTER TABLE embeddings ADD COLUMN IF NOT EXISTS embedding_vec vector`); err != nil {
		return err
	}

	var lastID int64
	for {
		var rows []struct {
			ID        int64  `db:"id"`
			Embedding []byte `db:"embedding"`
		}
		if err := tx.Select(&rows, `SELECT id, embedding FROM embeddings WHERE id > $1 ORDER BY id LIMIT $2`, lastID, v2_37_0Batch); err != nil {
			return fmt.Errorf("reading embeddings: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		ids := make([]int64, len(rows))
		vecs := make([]string, len(rows))
		dims := make([]int64, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
			vecs[i], dims[i] = bytesToVectorLiteral(r.Embedding), int64(len(r.Embedding)/4)
		}
		if _, err := tx.Exec(`
			UPDATE embeddings e SET embedding_vec = v.vec::vector, dimensions = v.dims
			FROM unnest($1::bigint[], $2::text[], $3::int[]) AS v(id, vec, dims)
			WHERE e.id = v.id`, pq.Array(ids), pq.Array(vecs), pq.Array(dims)); err != nil {
			return fmt.Errorf("converting embeddings: %w", err)
		}
		lastID = rows[len(rows)-1].ID
	}

	for _, q := range []string{
		`ALTER TABLE embeddings DROP COLUMN embedding`,
		`ALTER TABLE embeddings RENAME COLUMN embedding_vec TO embedding`,
		`ALTER TABLE embeddings ALTER COLUMN embedding SET NOT NULL`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// bytesToVectorLiteral turns the little-endian float32 blob the old in-memory index stored into pgvector's text form.
func bytesToVectorLiteral(b []byte) string {
	buf := make([]byte, 0, len(b)*3+2)
	buf = append(buf, '[')
	for i := 0; i+4 <= len(b); i += 4 {
		if i > 0 {
			buf = append(buf, ',')
		}
		f := math.Float32frombits(binary.LittleEndian.Uint32(b[i:]))
		buf = strconv.AppendFloat(buf, float64(f), 'g', -1, 32)
	}
	return string(append(buf, ']'))
}
