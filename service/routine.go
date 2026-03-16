package service

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	routineDailyRegex  = regexp.MustCompile(`^(\d{2}):(\d{2})\s*(.*)$`)
	routineWeeklyRegex = regexp.MustCompile(`^(월|화|수|목|금|토|일)\s+(\d{2}):(\d{2})\s*(.*)$`)
)

var weekdayMap = map[string]int{
	"일": 0, "월": 1, "화": 2, "수": 3, "목": 4, "금": 5, "토": 6,
}

// Routine represents a recurring routine row.
type Routine struct {
	Id            int64
	ChatID        int64
	ScheduleType  string // "daily" or "weekly"
	ScheduleParam string
	Message       string
	CreatedAt     time.Time
}

// RoutineService handles routine CRUD and due-time resolution.
type RoutineService struct {
	db *sql.DB
}

// NewRoutineService returns a RoutineService.
func NewRoutineService(db *sql.DB) *RoutineService {
	return &RoutineService{db: db}
}

// parseRoutineInput parses "HH:mm 메시지" (daily) or "요일 HH:mm 메시지" (weekly).
// Returns scheduleType, scheduleParam, message, error.
func parseRoutineInput(text string) (scheduleType, scheduleParam, message string, err error) {
	text = strings.TrimSpace(text)
	if m := routineWeeklyRegex.FindStringSubmatch(text); m != nil {
		wd, ok := weekdayMap[m[1]]
		if !ok {
			return "", "", "", fmt.Errorf("invalid weekday: %s", m[1])
		}
		h, _ := strconv.Atoi(m[2])
		min, _ := strconv.Atoi(m[3])
		if h < 0 || h > 23 || min < 0 || min > 59 {
			return "", "", "", fmt.Errorf("invalid time: hour 0-23, minute 0-59")
		}
		param := fmt.Sprintf("%d,%02d:%02d", wd, h, min)
		return "weekly", param, strings.TrimSpace(m[4]), nil
	}
	if m := routineDailyRegex.FindStringSubmatch(text); m != nil {
		h, _ := strconv.Atoi(m[1])
		min, _ := strconv.Atoi(m[2])
		if h < 0 || h > 23 || min < 0 || min > 59 {
			return "", "", "", fmt.Errorf("invalid time: hour 0-23, minute 0-59")
		}
		param := fmt.Sprintf("%02d:%02d", h, min)
		return "daily", param, strings.TrimSpace(m[3]), nil
	}
	return "", "", "", fmt.Errorf("invalid format: use HH:mm 메시지 or 요일 HH:mm 메시지 (예: 09:00 물 마시기, 월 08:00 회의)")
}

// Create parses routine input, inserts and returns id.
func (s *RoutineService) Create(chatID int64, text string, now time.Time) (int64, error) {
	typ, param, message, err := parseRoutineInput(text)
	if err != nil {
		return 0, err
	}
	nowSeoul := now.In(seoul)
	if now.IsZero() {
		nowSeoul = time.Now().In(seoul)
	}
	createdAt := nowSeoul
	res, err := s.db.Exec(
		`INSERT INTO routines (chat_id, schedule_type, schedule_param, message, created_at) VALUES (?, ?, ?, ?, ?)`,
		chatID, typ, param, message, formatTime(createdAt),
	)
	if err != nil {
		return 0, fmt.Errorf("insert: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// List returns all routines for the chat, ordered by id.
func (s *RoutineService) List(chatID int64) ([]Routine, error) {
	rows, err := s.db.Query(
		`SELECT id, chat_id, schedule_type, schedule_param, message, created_at FROM routines WHERE chat_id = ? ORDER BY id`,
		chatID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Routine
	for rows.Next() {
		var r Routine
		var createdAt string
		if err := rows.Scan(&r.Id, &r.ChatID, &r.ScheduleType, &r.ScheduleParam, &r.Message, &createdAt); err != nil {
			return nil, err
		}
		t, _ := parseTime(createdAt)
		r.CreatedAt = t
		list = append(list, r)
	}
	return list, rows.Err()
}

// ListDue returns routines that are due at the given time (Asia/Seoul).
func (s *RoutineService) ListDue(now time.Time) ([]Routine, error) {
	nowSeoul := now.In(seoul)
	currentHMM := nowSeoul.Format("15:04")
	weekday := int(nowSeoul.Weekday())

	rows, err := s.db.Query(`SELECT id, chat_id, schedule_type, schedule_param, message, created_at FROM routines`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var due []Routine
	for rows.Next() {
		var r Routine
		var createdAt string
		if err := rows.Scan(&r.Id, &r.ChatID, &r.ScheduleType, &r.ScheduleParam, &r.Message, &createdAt); err != nil {
			return nil, err
		}
		t, _ := parseTime(createdAt)
		r.CreatedAt = t
		if r.ScheduleType == "daily" && r.ScheduleParam == currentHMM {
			due = append(due, r)
			continue
		}
		if r.ScheduleType == "weekly" {
			parts := strings.SplitN(r.ScheduleParam, ",", 2)
			if len(parts) == 2 {
				w, _ := strconv.Atoi(parts[0])
				if w == weekday && parts[1] == currentHMM {
					due = append(due, r)
				}
			}
		}
	}
	return due, rows.Err()
}

// DeleteByListIndex deletes the routine at 1-based list index for the chat.
func (s *RoutineService) DeleteByListIndex(chatID int64, index int) error {
	list, err := s.List(chatID)
	if err != nil {
		return err
	}
	if index < 1 || index > len(list) {
		return fmt.Errorf("번호는 1~%d 사이여야 합니다", len(list))
	}
	id := list[index-1].Id
	_, err = s.db.Exec(`DELETE FROM routines WHERE id = ? AND chat_id = ?`, id, chatID)
	return err
}
