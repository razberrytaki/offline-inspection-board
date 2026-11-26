package inspection

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Outcome uint8

const (
	Created Outcome = iota + 1
	Replayed
	ContentMismatch
	RevisionTaken
	MissingPrevious
	Forbidden
)

var ErrUnknownEquipment = errors.New("등록되지 않은 설비입니다")

type Record struct {
	Submission
	ReceivedAt time.Time `json:"receivedAt"`
}

type Store struct {
	Pool *pgxpool.Pool
}

func (s Store) Put(ctx context.Context, submission Submission) (Outcome, Record, error) {
	var outcome Outcome
	var record Record
	err := pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		var err error
		outcome, record, err = put(ctx, tx, submission)
		return err
	})
	if err != nil {
		return 0, Record{}, err
	}
	return outcome, record, nil
}

func put(ctx context.Context, tx pgx.Tx, submission Submission) (Outcome, Record, error) {
	hash := submission.Hash()
	record, storedHash, err := findRecord(ctx, tx, submission.ID)
	if err == nil {
		return compareHash(hash[:], storedHash), record, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, Record{}, err
	}

	var equipmentExists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM equipment WHERE id = $1)", submission.EquipmentID).Scan(&equipmentExists)
	if err != nil {
		return 0, Record{}, err
	}
	if !equipmentExists {
		return 0, Record{}, ErrUnknownEquipment
	}

	var previousReceivedAt *time.Time
	if submission.Revision > 0 {
		var author string
		err = tx.QueryRow(ctx, "SELECT author_id FROM inspections WHERE id = $1 AND revision = 0", submission.RootID).Scan(&author)
		if errors.Is(err, pgx.ErrNoRows) {
			return MissingPrevious, Record{}, nil
		}
		if err != nil {
			return 0, Record{}, err
		}
		if author != submission.AuthorID {
			return Forbidden, Record{}, nil
		}

		err = tx.QueryRow(ctx, `
			SELECT received_at FROM inspections WHERE root_id = $1 AND revision = $2
		`, submission.RootID, submission.Revision-1).Scan(&previousReceivedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return MissingPrevious, Record{}, nil
		}
		if err != nil {
			return 0, Record{}, err
		}
	}

	var receivedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO inspections (
			id, root_id, revision, equipment_id, result, memo, inspected_at, author_id, content_hash, received_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			GREATEST(clock_timestamp(), $10::timestamptz + interval '1 microsecond')
		)
		ON CONFLICT DO NOTHING
		RETURNING received_at
	`, submission.ID, submission.RootID, submission.Revision, submission.EquipmentID, submission.Result,
		submission.Memo, time.UnixMilli(submission.InspectedAt).UTC(), submission.AuthorID, hash[:],
		previousReceivedAt).Scan(&receivedAt)
	if err == nil {
		return Created, Record{Submission: submission, ReceivedAt: receivedAt.UTC()}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, Record{}, err
	}

	// READ COMMITTED에서는 새 SELECT가 INSERT와 경합한 트랜잭션의 커밋을 볼 수 있다.
	record, storedHash, err = findRecord(ctx, tx, submission.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RevisionTaken, Record{}, nil
	}
	if err != nil {
		return 0, Record{}, err
	}
	return compareHash(hash[:], storedHash), record, nil
}

func findRecord(ctx context.Context, tx pgx.Tx, id string) (Record, []byte, error) {
	var record Record
	var inspectedAt time.Time
	var hash []byte
	err := tx.QueryRow(ctx, `
		SELECT id::text, root_id::text, revision, equipment_id, result, memo, inspected_at,
			author_id, content_hash, received_at
		FROM inspections WHERE id = $1
	`, id).Scan(&record.ID, &record.RootID, &record.Revision, &record.EquipmentID, &record.Result,
		&record.Memo, &inspectedAt, &record.AuthorID, &hash, &record.ReceivedAt)
	if err != nil {
		return Record{}, nil, err
	}
	record.InspectedAt = inspectedAt.UnixMilli()
	record.ReceivedAt = record.ReceivedAt.UTC()
	return record, hash, nil
}

func compareHash(sent, stored []byte) Outcome {
	if bytes.Equal(sent, stored) {
		return Replayed
	}
	return ContentMismatch
}
