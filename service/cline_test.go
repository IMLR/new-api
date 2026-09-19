package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cline"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClineRefreshPersistsRotationAndCoalescesConcurrentRequests(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB; sqlDB.Close() })
	InitHttpClient()
	var calls atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		var body map[string]string
		require.NoError(t, common.DecodeJson(r.Body, &body))
		assert.Equal(t, "old-refresh", body["refreshToken"])
		fmt.Fprintf(w, `{"success":true,"data":{"accessToken":"new-access","refreshToken":"new-refresh","expiresAt":%q}}`, time.Now().Add(time.Hour).Format(time.RFC3339))
	}))
	defer server.Close()
	ch := model.Channel{Type: constant.ChannelTypeCline, Key: `{"refreshToken":"old-refresh","accessToken":"old-access","expiresAt":1}`, BaseURL: &server.URL}
	require.NoError(t, db.Create(&ch).Error)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ResolveClineCredential(context.Background(), ch.Id, "")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), calls.Load())
	var saved model.Channel
	require.NoError(t, db.First(&saved, ch.Id).Error)
	credential, err := cline.ParseCredential(saved.Key)
	require.NoError(t, err)
	assert.Equal(t, "new-refresh", credential.RefreshToken)
	// A simultaneous 401 against the old token reuses the winner's new token.
	_, err = ResolveClineCredential(context.Background(), ch.Id, "old-access")
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
	fail.Store(true)
	_, err = ResolveClineCredential(context.Background(), ch.Id, "new-access")
	require.Error(t, err)
	var after model.Channel
	require.NoError(t, db.First(&after, ch.Id).Error)
	assert.Equal(t, saved.Key, after.Key)
}
