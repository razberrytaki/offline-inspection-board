package board

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Row struct {
	RootID        string    `json:"rootId"`
	Revision      int32     `json:"revision"`
	EquipmentID   string    `json:"equipmentId"`
	EquipmentName string    `json:"equipmentName"`
	Result        string    `json:"result"`
	Memo          string    `json:"memo"`
	AuthorID      string    `json:"authorId"`
	AuthorName    string    `json:"authorName"`
	InspectedAt   int64     `json:"inspectedAt"`
	ReceivedAt    time.Time `json:"receivedAt"`
}

type revision struct {
	ID string `json:"id"`
	Row
}

const columns = `i.id::text, i.root_id::text, i.revision, i.equipment_id, e.name,
	i.result, i.memo, i.author_id, u.name, i.inspected_at, i.received_at`

const names = `
	JOIN equipment e ON e.id = i.equipment_id
	JOIN users u ON u.id = i.author_id`

const listQuery = `SELECT ` + columns + `
	FROM (
		SELECT DISTINCT ON (root_id) *
		FROM inspections
		ORDER BY root_id, revision DESC
	) i` + names + `
	WHERE NOT $1::boolean OR i.result = 'abnormal'
	ORDER BY i.received_at DESC, i.root_id`

const revisionsForRoot = `SELECT ` + columns + `
	FROM inspections i` + names + `
	WHERE i.root_id = $1`

func readRevisions(ctx context.Context, pool *pgxpool.Pool, query string, arg any) ([]revision, error) {
	rows, err := pool.Query(ctx, query, arg)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanRevision)
}

func scanRevision(row pgx.CollectableRow) (revision, error) {
	var record revision
	var inspectedAt time.Time
	err := row.Scan(&record.ID, &record.RootID, &record.Revision, &record.EquipmentID, &record.EquipmentName,
		&record.Result, &record.Memo, &record.AuthorID, &record.AuthorName, &inspectedAt, &record.ReceivedAt)
	if err != nil {
		return revision{}, err
	}
	record.InspectedAt = inspectedAt.UnixMilli()
	record.ReceivedAt = record.ReceivedAt.UTC()
	return record, nil
}
