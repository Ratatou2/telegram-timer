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
	hhmmRegex     = regexp.MustCompile(`^(\d{2}):(\d{2})\s*(.*)$`)
	mmddHhmmRegex = regexp.MustCompile(`^(\d{2})/(\d{2})\s+(\d{2}):(\d{2})\s*(.*)$`)
	seoul         *time.Location
)

func init() {
	var err error
	seoul, err = time.LoadLocation("Asia/Seoul")
	if err != nil {
		seoul = time.UTC
	}
}

// Reminder represents a single reminder row.
type Reminder struct {
	Id        int64
	ChatID    int64
	RemindAt  time.Time
	Message   string
	Sent      bool
	CreatedAt time.Time
	SentAt    sql.NullTime
	Sent30m   bool
	Sent10m   bool
	Sent5m    bool
}

// ReminderService handles reminder CRUD and validation.
type ReminderService struct {
	db     *sql.DB
	nowFunc func() time.Time
}

// NewReminderService returns a ReminderService. nowFunc defaults to time.Now.
func NewReminderService(db *sql.DB) *ReminderService {
	svc := &ReminderService{db: db, nowFunc: time.Now}
	return svc
}

// parseRemindTime parses "HH:mm" and optional message from text. today is used for date.
// Returns hour, minute, message (rest after HH:mm), error.
func parseRemindTime(text string, today time.Time) (hour, minute int, message string, err error) {
	m := hhmmRegex.FindStringSubmatch(text)
	if m == nil {
		return 0, 0, "", fmt.Errorf("invalid format: use HH:mm 메시지")
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	if h < 0 || h > 23 || min < 0 || min > 59 {
		return 0, 0, "", fmt.Errorf("invalid time: hour 0-23, minute 0-59")
	}
	msg := m[3]
	return h, min, msg, nil
}

// resolveNextOccurrenceWithinOneYear returns the next occurrence of (month, day, hour, min)
// that is >= now and <= now+1 year in Asia/Seoul. Used for MM/dd HH:mm (no year).
func resolveNextOccurrenceWithinOneYear(month, day, hour, min int, now time.Time) (time.Time, error) {
	nowSeoul := now.In(seoul)
	oneYearLater := nowSeoul.AddDate(1, 0, 0)
	thisYear := time.Date(nowSeoul.Year(), time.Month(month), day, hour, min, 0, 0, seoul)
	nextYear := time.Date(nowSeoul.Year()+1, time.Month(month), day, hour, min, 0, 0, seoul)
	if thisYear.After(nowSeoul) && !thisYear.After(oneYearLater) {
		return thisYear, nil
	}
	if nextYear.After(nowSeoul) && !nextYear.After(oneYearLater) {
		return nextYear, nil
	}
	return time.Time{}, fmt.Errorf("해당 일시가 오늘 기준 1년을 넘어갑니다")
}

// parseRemindInput parses either "MM/dd HH:mm 메시지" or "HH:mm 메시지".
// For MM/dd: resolves to the next occurrence within 1 year from now.
// For HH:mm only: uses today's date (must be in the future).
// Returns remindAt, message, error.
func parseRemindInput(text string, now time.Time) (remindAt time.Time, message string, err error) {
	nowSeoul := now.In(seoul)
	text = strings.TrimSpace(text)
	// Try MM/dd HH:mm first
	if m := mmddHhmmRegex.FindStringSubmatch(text); m != nil {
		mo, _ := strconv.Atoi(m[1])
		d, _ := strconv.Atoi(m[2])
		h, _ := strconv.Atoi(m[3])
		min, _ := strconv.Atoi(m[4])
		msg := strings.TrimSpace(m[5])
		if mo < 1 || mo > 12 || d < 1 || d > 31 || h < 0 || h > 23 || min < 0 || min > 59 {
			return time.Time{}, "", fmt.Errorf("invalid date or time: MM 01-12, dd 01-31, HH 00-23, mm 00-59")
		}
		remindAt, err = resolveNextOccurrenceWithinOneYear(mo, d, h, min, nowSeoul)
		if err != nil {
			return time.Time{}, "", err
		}
		return remindAt, msg, nil
	}
	// Fall back to HH:mm (today only)
	hour, min, msg, err := parseRemindTime(text, nowSeoul)
	if err != nil {
		return time.Time{}, "", err
	}
	remindAt = time.Date(nowSeoul.Year(), nowSeoul.Month(), nowSeoul.Day(), hour, min, 0, 0, seoul)
	if !remindAt.After(nowSeoul) {
		return time.Time{}, "", fmt.Errorf("remind time has already passed")
	}
	return remindAt, msg, nil
}

// Create parses "MM/dd HH:mm 메시지" or "HH:mm 메시지", validates (within 1 year / today not past), inserts and returns id.
// All times are interpreted in Asia/Seoul. MM/dd resolves to the next occurrence within 1 year from now.
func (s *ReminderService) Create(chatID int64, text string, now time.Time) (int64, error) {
	nowSeoul := now.In(seoul)
	if now.IsZero() {
		nowSeoul = s.nowFunc().In(seoul)
	}
	remindAt, message, err := parseRemindInput(text, nowSeoul)
	if err != nil {
		return 0, err
	}
	createdAt := nowSeoul
	res, err := s.db.Exec(
		`INSERT INTO reminders (chat_id, remind_at, message, sent, created_at) VALUES (?, ?, ?, 0, ?)`,
		chatID, formatTime(remindAt), message, formatTime(createdAt),
	)
	if err != nil {
		return 0, fmt.Errorf("insert: %w", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// ListUnsent returns unsent reminders for chat with remind_at >= now (Asia/Seoul), ordered by remind_at.
func (s *ReminderService) ListUnsent(chatID int64, now time.Time) ([]Reminder, error) {
	nowSeoul := now.In(seoul)
	rows, err := s.db.Query(
		`SELECT id, chat_id, strftime('%Y-%m-%d %H:%M:%S', remind_at) as remind_at, message, sent,
		 strftime('%Y-%m-%d %H:%M:%S', created_at) as created_at, sent_at, sent_30m, sent_10m, sent_5m FROM reminders
		 WHERE chat_id = ? AND sent = 0 AND remind_at >= ?
		 ORDER BY remind_at`,
		chatID, formatTime(nowSeoul),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Reminder
	for rows.Next() {
		var r Reminder
		var remindAt, createdAt string
		var sent, sent30m, sent10m, sent5m int
		var sentAt sql.NullString
		if err := rows.Scan(&r.Id, &r.ChatID, &remindAt, &r.Message, &sent, &createdAt, &sentAt, &sent30m, &sent10m, &sent5m); err != nil {
			return nil, err
		}
		r.RemindAt, _ = parseTime(remindAt)
		r.CreatedAt, _ = parseTime(createdAt)
		r.Sent = sent != 0
		r.Sent30m = sent30m != 0
		r.Sent10m = sent10m != 0
		r.Sent5m = sent5m != 0
		if sentAt.Valid && sentAt.String != "" {
			t, _ := parseTime(sentAt.String)
			r.SentAt = sql.NullTime{Time: t, Valid: true}
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// DeleteByListIndex deletes the reminder at 1-based index in ListUnsent order.
func (s *ReminderService) DeleteByListIndex(chatID int64, index int, now time.Time) error {
	if index < 1 {
		return fmt.Errorf("invalid index: must be >= 1")
	}
	list, err := s.ListUnsent(chatID, now)
	if err != nil {
		return err
	}
	if index > len(list) {
		return fmt.Errorf("invalid index: list has %d items", len(list))
	}
	id := list[index-1].Id
	_, err = s.db.Exec(`DELETE FROM reminders WHERE id = ? AND chat_id = ?`, id, chatID)
	return err
}

// ListDue returns reminders where sent = false and remind_at <= now (Asia/Seoul).
func (s *ReminderService) ListDue(now time.Time) ([]Reminder, error) {
	nowSeoul := now.In(seoul)
	rows, err := s.db.Query(
		`SELECT id, chat_id, strftime('%Y-%m-%d %H:%M:%S', remind_at) as remind_at, message, sent,
		 strftime('%Y-%m-%d %H:%M:%S', created_at) as created_at, sent_at, sent_30m, sent_10m, sent_5m FROM reminders
		 WHERE sent = 0 AND remind_at <= ?
		 ORDER BY remind_at`,
		formatTime(nowSeoul),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Reminder
	for rows.Next() {
		var r Reminder
		var remindAt, createdAt string
		var sent, sent30m, sent10m, sent5m int
		var sentAt sql.NullString
		if err := rows.Scan(&r.Id, &r.ChatID, &remindAt, &r.Message, &sent, &createdAt, &sentAt, &sent30m, &sent10m, &sent5m); err != nil {
			return nil, err
		}
		r.RemindAt, _ = parseTime(remindAt)
		r.CreatedAt, _ = parseTime(createdAt)
		r.Sent = sent != 0
		r.Sent30m = sent30m != 0
		r.Sent10m = sent10m != 0
		r.Sent5m = sent5m != 0
		if sentAt.Valid && sentAt.String != "" {
			t, _ := parseTime(sentAt.String)
			r.SentAt = sql.NullTime{Time: t, Valid: true}
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// ListDueAdvance returns reminders that are due for an advance notification: sent=0, remind_at in the future
// but within advanceMinutes from now, and the corresponding advance flag not yet set.
// advanceMinutes must be 30, 10, or 5.
func (s *ReminderService) ListDueAdvance(now time.Time, advanceMinutes int) ([]Reminder, error) {
	nowSeoul := now.In(seoul)
	windowEnd := nowSeoul.Add(time.Duration(advanceMinutes) * time.Minute)
	col := advanceColumn(advanceMinutes)
	if col == "" {
		return nil, fmt.Errorf("invalid advanceMinutes: want 30, 10, or 5, got %d", advanceMinutes)
	}
	rows, err := s.db.Query(
		`SELECT id, chat_id, strftime('%Y-%m-%d %H:%M:%S', remind_at) as remind_at, message, sent,
		 strftime('%Y-%m-%d %H:%M:%S', created_at) as created_at, sent_at, sent_30m, sent_10m, sent_5m FROM reminders
		 WHERE sent = 0 AND remind_at > ? AND remind_at <= ? AND `+col+` = 0
		 ORDER BY remind_at`,
		formatTime(nowSeoul), formatTime(windowEnd),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Reminder
	for rows.Next() {
		var r Reminder
		var remindAt, createdAt string
		var sent, sent30m, sent10m, sent5m int
		var sentAt sql.NullString
		if err := rows.Scan(&r.Id, &r.ChatID, &remindAt, &r.Message, &sent, &createdAt, &sentAt, &sent30m, &sent10m, &sent5m); err != nil {
			return nil, err
		}
		r.RemindAt, _ = parseTime(remindAt)
		r.CreatedAt, _ = parseTime(createdAt)
		r.Sent = sent != 0
		r.Sent30m = sent30m != 0
		r.Sent10m = sent10m != 0
		r.Sent5m = sent5m != 0
		if sentAt.Valid && sentAt.String != "" {
			t, _ := parseTime(sentAt.String)
			r.SentAt = sql.NullTime{Time: t, Valid: true}
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func advanceColumn(advanceMinutes int) string {
	switch advanceMinutes {
	case 30:
		return "sent_30m"
	case 10:
		return "sent_10m"
	case 5:
		return "sent_5m"
	default:
		return ""
	}
}

// MarkSent30m sets sent_30m = 1 for the reminder.
func (s *ReminderService) MarkSent30m(id int64) error {
	_, err := s.db.Exec(`UPDATE reminders SET sent_30m = 1 WHERE id = ?`, id)
	return err
}

// MarkSent10m sets sent_10m = 1 for the reminder.
func (s *ReminderService) MarkSent10m(id int64) error {
	_, err := s.db.Exec(`UPDATE reminders SET sent_10m = 1 WHERE id = ?`, id)
	return err
}

// MarkSent5m sets sent_5m = 1 for the reminder.
func (s *ReminderService) MarkSent5m(id int64) error {
	_, err := s.db.Exec(`UPDATE reminders SET sent_5m = 1 WHERE id = ?`, id)
	return err
}

// MarkSent sets sent = 1 and sent_at for the reminder (Asia/Seoul).
func (s *ReminderService) MarkSent(id int64, sentAt time.Time) error {
	_, err := s.db.Exec(`UPDATE reminders SET sent = 1, sent_at = ? WHERE id = ?`, formatTime(sentAt.In(seoul)), id)
	return err
}

const timeLayout = "2006-01-02 15:04:05"

// timeLayouts: order matters. First match wins.
// - timeLayout: what we store (formatTime)
// - time.String() format: driver may return time.Time which becomes "2006-01-02 15:04:05 +0900 KST"
// - RFC3339, ISO variants: driver or SQLite may return these
var timeLayouts = []string{
	timeLayout,                      // "2006-01-02 15:04:05"
	"2006-01-02 15:04:05 -0700 MST", // time.Time.String() e.g. "2026-02-06 10:06:00 +0900 KST"
	time.RFC3339,                    // "2006-01-02T15:04:05Z07:00"
	"2006-01-02T15:04:05Z",          // UTC with Z
	"2006-01-02T15:04:05.000Z",      // UTC with milliseconds
	"2006-01-02 15:04:05.000",       // with milliseconds
}

func formatTime(t time.Time) string {
	return t.Format(timeLayout)
}

func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range timeLayouts {
		if t, err := time.ParseInLocation(layout, s, seoul); err == nil {
			return t, nil
		}
		if t, err := time.Parse(layout, s); err == nil {
			return t.In(seoul), nil
		}
	}
	return time.ParseInLocation(timeLayout, s, seoul)
}
