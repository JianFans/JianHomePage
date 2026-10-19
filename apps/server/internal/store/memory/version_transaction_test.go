package memory_test

import (
	"context"
	"errors"
	"testing"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/store/memory"
)

// TestContentTransactionRollsBackFailedCloneAudit ensures a failed transaction
// cannot leave a draft that would appear successful in subsequent library reads.
func TestContentTransactionRollsBackFailedCloneAudit(t *testing.T) {
	repo := memory.NewContentRepository(memory.NewState())
	failure := errors.New("audit failed")
	err := repo.WithinTransaction(context.Background(), func(tx content.Repository) error {
		if err := tx.CreateVersion(context.Background(), domain.ContentVersion{ID: "ver_a62ad864-696a-44b4-9bc8-bc7cc3120541"}); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if _, err := repo.GetVersion(context.Background(), "ver_a62ad864-696a-44b4-9bc8-bc7cc3120541"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("transaction did not roll back")
	}
}
