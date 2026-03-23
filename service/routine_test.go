package service

import (
	"testing"
	"time"
)

func TestParseRoutineInput_Daily(t *testing.T) {
	typ, param, msg, err := parseRoutineInput("09:00 물 마시기")
	if err != nil {
		t.Fatalf("parseRoutineInput: %v", err)
	}
	if typ != "daily" || param != "09:00" || msg != "물 마시기" {
		t.Errorf("got %q %q %q", typ, param, msg)
	}
}

func TestParseRoutineInput_WeeklySingle(t *testing.T) {
	typ, param, msg, err := parseRoutineInput("월 08:00 주간 회의")
	if err != nil {
		t.Fatalf("parseRoutineInput: %v", err)
	}
	if typ != "weekly" || param != "1,08:00" || msg != "주간 회의" {
		t.Errorf("got %q %q %q", typ, param, msg)
	}
}

func TestParseRoutineInput_WeeklyCommaDays(t *testing.T) {
	typ, param, msg, err := parseRoutineInput("월, 수, 금 12:00 약")
	if err != nil {
		t.Fatalf("parseRoutineInput: %v", err)
	}
	if typ != "weekly" || param != "1,3,5,12:00" || msg != "약" {
		t.Errorf("got %q %q %q", typ, param, msg)
	}
}

func TestParseRoutineInput_WeeklyRange(t *testing.T) {
	typ, param, msg, err := parseRoutineInput("월-금 18:00 퇴근")
	if err != nil {
		t.Fatalf("parseRoutineInput: %v", err)
	}
	if typ != "weekly" || param != "1,2,3,4,5,18:00" || msg != "퇴근" {
		t.Errorf("got %q %q %q", typ, param, msg)
	}
}

func TestParseRoutineInput_WeeklySatSun(t *testing.T) {
	typ, param, msg, err := parseRoutineInput("토-일 10:00 브런치")
	if err != nil {
		t.Fatalf("parseRoutineInput: %v", err)
	}
	if typ != "weekly" || param != "0,6,10:00" || msg != "브런치" {
		t.Errorf("got %q %q %q", typ, param, msg)
	}
}

func TestParseRoutineInput_WeekdayKeywords(t *testing.T) {
	typ, param, msg, err := parseRoutineInput("평일 09:00 출근")
	if err != nil {
		t.Fatalf("parseRoutineInput: %v", err)
	}
	if typ != "weekly" || param != "1,2,3,4,5,09:00" || msg != "출근" {
		t.Errorf("got %q %q %q", typ, param, msg)
	}
	typ, param, msg, err = parseRoutineInput("주말 11:00 늦잠")
	if err != nil {
		t.Fatalf("parseRoutineInput: %v", err)
	}
	if typ != "weekly" || param != "0,6,11:00" || msg != "늦잠" {
		t.Errorf("got %q %q %q", typ, param, msg)
	}
}

func TestParseRoutineInput_ReverseRangeError(t *testing.T) {
	_, _, _, err := parseRoutineInput("금-월 09:00 x")
	if err == nil {
		t.Fatal("expected error for reverse range")
	}
}

func TestParseRoutineInput_InvalidPrefixToken(t *testing.T) {
	_, _, _, err := parseRoutineInput("회의 09:00 내용")
	if err == nil {
		t.Fatal("expected error for invalid weekday prefix")
	}
}

func TestParseRoutineInput_InvalidTime(t *testing.T) {
	_, _, _, err := parseRoutineInput("월 25:00 x")
	if err == nil {
		t.Fatal("expected error for invalid time")
	}
}

func TestRoutineService_ListDue_WeeklyMulti(t *testing.T) {
	d := setupTestDB(t)
	svc := NewRoutineService(d)
	loc := seoul
	now := time.Date(2025, 3, 3, 12, 0, 0, 0, loc) // Monday 12:00 Seoul

	_, err := svc.db.Exec(
		`INSERT INTO routines (chat_id, schedule_type, schedule_param, message, created_at) VALUES (?, ?, ?, ?, ?)`,
		1, "weekly", "1,3,5,12:00", "약", formatTime(now),
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	due, err := svc.ListDue(time.Date(2025, 3, 5, 12, 0, 0, 0, loc)) // Wed 12:00
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(due) != 1 || due[0].Message != "약" {
		t.Errorf("ListDue Wed: got %+v", due)
	}

	due, err = svc.ListDue(time.Date(2025, 3, 4, 12, 0, 0, 0, loc)) // Tue 12:00
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("ListDue Tue: want 0, got %+v", due)
	}
}

func TestRoutineService_ListDue_LegacySingleWeekday(t *testing.T) {
	d := setupTestDB(t)
	svc := NewRoutineService(d)
	loc := seoul
	now := time.Date(2025, 3, 3, 10, 0, 0, 0, loc)

	_, err := svc.db.Exec(
		`INSERT INTO routines (chat_id, schedule_type, schedule_param, message, created_at) VALUES (?, ?, ?, ?, ?)`,
		1, "weekly", "1,08:00", "레거시", formatTime(now),
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	due, err := svc.ListDue(time.Date(2025, 3, 3, 8, 0, 0, 0, loc))
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(due) != 1 {
		t.Errorf("got %d due", len(due))
	}
}
