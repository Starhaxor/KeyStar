package domain

import (
	"testing"
	"time"
)

func TestDeviceBanIsActiveOnlyBeforeExpiry(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	if !(&DeviceBan{Status: BanStatusActive, ExpiresAt: &future}).IsActiveAt(now) {
		t.Fatal("active future device ban must block verification")
	}
	if (&DeviceBan{Status: BanStatusActive, ExpiresAt: &past}).IsActiveAt(now) {
		t.Fatal("expired device ban must not block verification")
	}
	if (&DeviceBan{Status: BanStatusLifted}).IsActiveAt(now) {
		t.Fatal("lifted device ban must not block verification")
	}
}
