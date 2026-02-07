package service

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"telegram-timer/db"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestParseRemindTime(t *testing.T) {
	loc := time.Local
	today := time.Date(2025, 2, 5, 12, 0, 0, 0, loc)

	tests := []struct {
		name    string
		text    string
		wantErr bool
		wantH   int
		wantM   int
	}{
		{"valid", "15:30 회의", false, 15, 30},
		{"valid no message", "09:00", false, 9, 0},
		{"invalid format", "9:00 x", true, 0, 0},
		{"invalid hour", "25:00 x", true, 0, 0},
		{"invalid minute", "12:60 x", true, 0, 0},
		{"no time", "회의", true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, m, msg, err := parseRemindTime(tt.text, today)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseRemindTime() err = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if h != tt.wantH || m != tt.wantM {
				t.Errorf("parseRemindTime() got %d:%d, want %d:%d", h, m, tt.wantH, tt.wantM)
			}
			_ = msg
		})
	}
}

func TestCreate_Valid(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	loc := time.Local
	// Set "now" to 10:00 so 15:30 is in the future
	now := time.Date(2025, 2, 5, 10, 0, 0, 0, loc)
	svc.nowFunc = func() time.Time { return now }

	id, err := svc.Create(123, "15:30 회의 일정", now)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id <= 0 {
		t.Errorf("Create: id = %d", id)
	}

	list, err := svc.ListUnsent(123, now)
	if err != nil {
		t.Fatalf("ListUnsent: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListUnsent: len = %d", len(list))
	}
	if list[0].Message != "회의 일정" {
		t.Errorf("ListUnsent[0].Message = %q", list[0].Message)
	}
}

func TestCreate_PastTime_Error(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	now := time.Date(2025, 2, 5, 16, 0, 0, 0, time.Local)
	svc.nowFunc = func() time.Time { return now }

	_, err := svc.Create(123, "15:30 회의", now)
	if err == nil {
		t.Fatal("Create past time: expected error")
	}
}

func TestCreate_InvalidFormat_Error(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	now := time.Date(2025, 2, 5, 10, 0, 0, 0, time.Local)
	svc.nowFunc = func() time.Time { return now }

	_, err := svc.Create(123, "25:00 잘못된 시간", now)
	if err == nil {
		t.Fatal("Create invalid format: expected error")
	}
}

func TestListUnsent_Order(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	now := time.Date(2025, 2, 5, 8, 0, 0, 0, time.Local)
	svc.nowFunc = func() time.Time { return now }

	_, _ = svc.Create(1, "10:00 첫번째", now)
	_, _ = svc.Create(1, "09:00 두번째", now)
	_, _ = svc.Create(1, "11:00 세번째", now)

	list, err := svc.ListUnsent(1, now)
	if err != nil {
		t.Fatalf("ListUnsent: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("ListUnsent: len = %d", len(list))
	}
	if list[0].Message != "두번째" || list[1].Message != "첫번째" || list[2].Message != "세번째" {
		t.Errorf("ListUnsent order: got %q, %q, %q", list[0].Message, list[1].Message, list[2].Message)
	}
}

func TestDeleteByListIndex(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	now := time.Date(2025, 2, 5, 8, 0, 0, 0, time.Local)
	svc.nowFunc = func() time.Time { return now }

	_, _ = svc.Create(1, "10:00 A", now)
	_, _ = svc.Create(1, "11:00 B", now)

	err := svc.DeleteByListIndex(1, 1, now)
	if err != nil {
		t.Fatalf("DeleteByListIndex: %v", err)
	}
	list, _ := svc.ListUnsent(1, now)
	if len(list) != 1 || list[0].Message != "B" {
		t.Errorf("after delete index 1: got %v", list)
	}
}

func TestDeleteByListIndex_OutOfRange_Error(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	now := time.Date(2025, 2, 5, 8, 0, 0, 0, time.Local)
	svc.nowFunc = func() time.Time { return now }
	_, _ = svc.Create(1, "10:00 A", now)

	err := svc.DeleteByListIndex(1, 0, now)
	if err == nil {
		t.Fatal("DeleteByListIndex(0): expected error")
	}
	err = svc.DeleteByListIndex(1, 2, now)
	if err == nil {
		t.Fatal("DeleteByListIndex(2): expected error")
	}
}

func TestListDue(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	createTime := time.Date(2025, 2, 5, 8, 0, 0, 0, time.Local)
	dueTime := time.Date(2025, 2, 5, 10, 0, 0, 0, time.Local)
	svc.nowFunc = func() time.Time { return createTime }

	_, _ = svc.Create(1, "09:00 과거", createTime)
	_, _ = svc.Create(1, "10:00 지금", createTime)
	_, _ = svc.Create(1, "11:00 미래", createTime)

	due, err := svc.ListDue(dueTime)
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("ListDue: len = %d, want 2", len(due))
	}
}

func TestMarkSent(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	now := time.Date(2025, 2, 5, 10, 0, 0, 0, time.Local)
	svc.nowFunc = func() time.Time { return now }

	id, _ := svc.Create(1, "09:00 알림", now)
	err := svc.MarkSent(id, now)
	if err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	due, _ := svc.ListDue(now)
	for _, r := range due {
		if r.Id == id {
			t.Error("MarkSent: reminder still in ListDue")
		}
	}
}

func TestListDueAdvance_30m(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	createTime := time.Date(2025, 2, 5, 9, 0, 0, 0, time.Local)
	svc.nowFunc = func() time.Time { return createTime }

	id, _ := svc.Create(1, "10:00 회의", createTime)
	// 09:31 = 29분 후, 아직 30분 전 창 밖
	adv, err := svc.ListDueAdvance(time.Date(2025, 2, 5, 9, 31, 0, 0, time.Local), 30)
	if err != nil {
		t.Fatalf("ListDueAdvance: %v", err)
	}
	if len(adv) != 1 || adv[0].Id != id {
		t.Fatalf("ListDueAdvance(30m) at 09:31: got %v", adv)
	}
	_ = svc.MarkSent30m(id)
	adv, _ = svc.ListDueAdvance(time.Date(2025, 2, 5, 9, 31, 0, 0, time.Local), 30)
	if len(adv) != 0 {
		t.Errorf("ListDueAdvance(30m) after MarkSent30m: got %d", len(adv))
	}
}

func TestListDueAdvance_10m_5m(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	createTime := time.Date(2025, 2, 5, 9, 0, 0, 0, time.Local)
	svc.nowFunc = func() time.Time { return createTime }

	id, _ := svc.Create(1, "10:00 회의", createTime)
	// 09:51 = 9분 후, 10분 전 창 안
	adv10, _ := svc.ListDueAdvance(time.Date(2025, 2, 5, 9, 51, 0, 0, time.Local), 10)
	if len(adv10) != 1 {
		t.Fatalf("ListDueAdvance(10m) at 09:51: want 1, got %d", len(adv10))
	}
	// 09:56 = 4분 후, 5분 전 창 안
	adv5, _ := svc.ListDueAdvance(time.Date(2025, 2, 5, 9, 56, 0, 0, time.Local), 5)
	if len(adv5) != 1 {
		t.Fatalf("ListDueAdvance(5m) at 09:56: want 1, got %d", len(adv5))
	}
	_ = svc.MarkSent10m(id)
	_ = svc.MarkSent5m(id)
	adv10, _ = svc.ListDueAdvance(time.Date(2025, 2, 5, 9, 51, 0, 0, time.Local), 10)
	adv5, _ = svc.ListDueAdvance(time.Date(2025, 2, 5, 9, 56, 0, 0, time.Local), 5)
	if len(adv10) != 0 || len(adv5) != 0 {
		t.Errorf("after MarkSent10m/5m: adv10=%d adv5=%d", len(adv10), len(adv5))
	}
}

func TestListDueAdvance_InvalidMinutes_Error(t *testing.T) {
	d := setupTestDB(t)
	svc := NewReminderService(d)
	now := time.Date(2025, 2, 5, 10, 0, 0, 0, time.Local)
	_, err := svc.ListDueAdvance(now, 15)
	if err == nil {
		t.Fatal("ListDueAdvance(15): expected error")
	}
}
