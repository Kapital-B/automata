package queries

import (
	"context"
	messages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	"github.com/google/uuid"
	"testing"
)

type summaryStore struct {
	messages.SummaryRepository
	row messages.SummarySettingsRow
}

func (f *summaryStore) UpsertSummarySettings(_ context.Context, row messages.SummarySettingsRow) error {
	f.row = row
	return nil
}
func TestSummarySettingsDefaultsBelongToUseCase(t *testing.T) {
	f := &summaryStore{}
	s := &Service{Summaries: f}
	user := uuid.New()
	for _, size := range []int{-1, 0, 24} {
		if err := s.SaveSummarySettings(context.Background(), user, []string{"action"}, []string{"spam"}, size); err != nil {
			t.Fatal(err)
		}
		want := size
		if want <= 0 {
			want = 12
		}
		if f.row.UserID != user || f.row.ChunkSize != want || f.row.UpdatedAt.IsZero() {
			t.Fatalf("%+v", f.row)
		}
	}
}
