package controller

import (
	"testing"
	"time"

	workbuddyapi "github.com/QuantumNous/new-api/pkg/workbuddy"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkBuddyQuotaPackagesMarkExpiringCredits(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	credits := &workbuddyapi.Credits{
		Remain: 300,
		Used:   100,
		Size:   400,
		Packages: []workbuddyapi.CreditPackage{
			{Name: "monthly", Remain: 200, Used: 100, Size: 300, EndTime: "2026-09-25 00:00:00"},
			{Name: "bonus", Remain: 100, Used: 0, Size: 100, EndTime: "2026-12-31 00:00:00"},
		},
	}

	packages := workBuddyQuotaPackages(credits, now)
	require.Len(t, packages, 2)
	assert.True(t, packages[0].ExpiresSoon, "a package inside the week is flagged")
	assert.Equal(t, "2026-09-25T00:00:00+08:00", packages[0].ExpiresAt, "package times are CST")
	assert.False(t, packages[1].ExpiresSoon)
	assert.Empty(t, workBuddyQuotaPackages(nil, now))
}

func TestWorkBuddyChannelRejectsOtherChannelTypes(t *testing.T) {
	assert.ErrorContains(t, errChannelNotWorkBuddy, "WorkBuddy")
	assert.ErrorContains(t, errWorkBuddyMultiKey, "multi-key")
}
