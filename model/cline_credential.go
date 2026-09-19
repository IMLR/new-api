package model

import (
	"context"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

// SQLite has no row-level FOR UPDATE. This local lock also serializes refreshes in a single process.
var clineCredentialLocks sync.Map

func WithClineCredentialLock(ctx context.Context, id int, update func(*Channel) (string, error)) error {
	entry, _ := clineCredentialLocks.LoadOrStore(id, &sync.Mutex{})
	lock := entry.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ch Channel
		if err := lockForUpdate(tx).First(&ch, id).Error; err != nil {
			return err
		}
		if ch.Type != constant.ChannelTypeCline || ch.ChannelInfo.IsMultiKey {
			return fmt.Errorf("Cline requires a single-credential channel")
		}
		next, err := update(&ch)
		if err != nil {
			return err
		}
		if next == "" || next == ch.Key {
			return nil
		}
		return tx.Model(&Channel{}).Where("id = ?", id).Update("key", next).Error
	})
}
