package inspection

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

const (
	minPostgresUnixMillis = -210_866_803_200_000
	endPostgresUnixMillis = 9_224_318_016_000_000
)

var errInvalidSubmission = errors.New("점검 요청 형식이 올바르지 않습니다")

type Content struct {
	EquipmentID string `json:"equipmentId"`
	Result      string `json:"result"`
	Memo        string `json:"memo"`
	InspectedAt int64  `json:"inspectedAt"`
	AuthorID    string `json:"authorId"`
	RootID      string `json:"rootId"`
	Revision    int32  `json:"revision"`
}

type Submission struct {
	ID string `json:"id"`
	Content
}

func Parse(id string, body []byte, author string) (Submission, error) {
	if !utf8.Valid(body) {
		return Submission{}, errInvalidSubmission
	}
	var input struct {
		EquipmentID string `json:"equipmentId"`
		Result      string `json:"result"`
		Memo        string `json:"memo"`
		InspectedAt *int64 `json:"inspectedAt"`
		RootID      string `json:"rootId"`
		Revision    *int32 `json:"revision"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || decoder.Decode(new(json.RawMessage)) != io.EOF {
		return Submission{}, errInvalidSubmission
	}

	inspectionID, idErr := uuid.Parse(id)
	rootID, rootErr := uuid.Parse(input.RootID)
	if len(id) != 36 || idErr != nil || len(input.RootID) != 36 || rootErr != nil {
		return Submission{}, errInvalidSubmission
	}
	if input.Revision == nil || *input.Revision < 0 || (*input.Revision == 0) != (inspectionID == rootID) {
		return Submission{}, errInvalidSubmission
	}
	if input.InspectedAt == nil || *input.InspectedAt < minPostgresUnixMillis || *input.InspectedAt >= endPostgresUnixMillis {
		return Submission{}, errInvalidSubmission
	}

	memo := strings.TrimSpace(norm.NFC.String(input.Memo))
	if input.EquipmentID == "" || strings.ContainsRune(input.EquipmentID, 0) || strings.ContainsRune(memo, 0) {
		return Submission{}, errInvalidSubmission
	}
	switch input.Result {
	case "normal":
	case "abnormal":
		if memo == "" {
			return Submission{}, errInvalidSubmission
		}
	default:
		return Submission{}, errInvalidSubmission
	}

	return Submission{
		ID: inspectionID.String(),
		Content: Content{
			EquipmentID: input.EquipmentID,
			Result:      input.Result,
			Memo:        memo,
			InspectedAt: *input.InspectedAt,
			AuthorID:    author,
			RootID:      rootID.String(),
			Revision:    *input.Revision,
		},
	}, nil
}

func (s Submission) Hash() [sha256.Size]byte {
	canonical, _ := json.Marshal(s.Content)
	return sha256.Sum256(canonical)
}
