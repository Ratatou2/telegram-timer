package service

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	routineTimeTokenRe = regexp.MustCompile(`\d{2}:\d{2}`)
	hyphenSpacedRe     = regexp.MustCompile(`\s*-\s*`)
)

var weekdayMap = map[rune]int{
	'일': 0, '월': 1, '화': 2, '수': 3, '목': 4, '금': 5, '토': 6,
}

// weekOrder is Mon → Sun (Korean weekday line). Indices align with range expansion.
var weekOrder = []int{1, 2, 3, 4, 5, 6, 0}

func dayIndexInWeekOrder(w int) int {
	for i, d := range weekOrder {
		if d == w {
			return i
		}
	}
	return -1
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

// findFirstScheduleTime returns the index of the first valid HH:mm token and its hour/minute.
func findFirstScheduleTime(s string) (idx, hour, minute int, ok bool) {
	for _, loc := range routineTimeTokenRe.FindAllStringIndex(s, -1) {
		sub := s[loc[0]:loc[1]]
		h, err1 := strconv.Atoi(sub[0:2])
		m, err2 := strconv.Atoi(sub[3:5])
		if err1 != nil || err2 != nil {
			continue
		}
		if h >= 0 && h <= 23 && m >= 0 && m <= 59 {
			return loc[0], h, m, true
		}
	}
	return 0, 0, 0, false
}

var rangeTokenRe = regexp.MustCompile(`^([일월화수목금토])-([일월화수목금토])$`)

func expandRange(startR, endR rune) ([]int, error) {
	s, ok1 := weekdayMap[startR]
	e, ok2 := weekdayMap[endR]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("invalid weekday in range")
	}
	ia, ib := dayIndexInWeekOrder(s), dayIndexInWeekOrder(e)
	if ia < 0 || ib < 0 {
		return nil, fmt.Errorf("invalid weekday in range")
	}
	if ia > ib {
		return nil, fmt.Errorf("역방향 범위는 허용하지 않습니다 (월→일 순만, 예: 월-금)")
	}
	return append([]int(nil), weekOrder[ia:ib+1]...), nil
}

func parseWeekdayToken(tok string) ([]int, error) {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return nil, nil
	}
	switch tok {
	case "평일":
		return []int{1, 2, 3, 4, 5}, nil
	case "주말":
		return []int{0, 6}, nil
	}
	if m := rangeTokenRe.FindStringSubmatch(tok); m != nil {
		r0, r1 := []rune(m[1]), []rune(m[2])
		if len(r0) != 1 || len(r1) != 1 {
			return nil, fmt.Errorf("invalid range: %s", tok)
		}
		return expandRange(r0[0], r1[0])
	}
	runes := []rune(tok)
	if len(runes) == 1 {
		w, ok := weekdayMap[runes[0]]
		if !ok {
			return nil, fmt.Errorf("알 수 없는 요일 토큰: %s", tok)
		}
		return []int{w}, nil
	}
	return nil, fmt.Errorf("알 수 없는 요일 토큰: %s", tok)
}

// parseWeekdayPrefix parses the leading weekday expression (before the first HH:mm).
// Allowed: comma/space-separated tokens, single days, 월-금 ranges (forward only), 평일, 주말.
func parseWeekdayPrefix(prefix string) ([]int, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil, nil
	}
	segments := strings.Split(prefix, ",")
	var out []int
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		seg = hyphenSpacedRe.ReplaceAllString(seg, "-")
		parts := strings.Fields(seg)
		if len(parts) == 0 {
			continue
		}
		for _, p := range parts {
			days, err := parseWeekdayToken(p)
			if err != nil {
				return nil, err
			}
			out = append(out, days...)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("요일을 지정해 주세요 (예: 월, 월-금, 평일)")
	}
	seen := make(map[int]struct{})
	var uniq []int
	for _, d := range out {
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		uniq = append(uniq, d)
	}
	sort.Ints(uniq)
	return uniq, nil
}

// parseRoutineInput parses "HH:mm 메시지" (daily) or "요일… HH:mm 메시지" (weekly).
// Weekly schedule_param is "w1,w2,...,HH:mm" with Go weekday numbers (일=0 … 토=6).
func parseRoutineInput(text string) (scheduleType, scheduleParam, message string, err error) {
	text = strings.TrimSpace(text)
	idx, h, min, ok := findFirstScheduleTime(text)
	if !ok {
		return "", "", "", fmt.Errorf("invalid format: 첫 번째 HH:mm 앞은 요일만, 뒤는 메시지입니다 (예: 09:00 물 마시기, 월-금 09:00 회의)")
	}
	message = strings.TrimSpace(text[idx+5:])
	prefix := strings.TrimSpace(text[:idx])
	hourMin := fmt.Sprintf("%02d:%02d", h, min)
	if prefix == "" {
		if h < 0 || h > 23 || min < 0 || min > 59 {
			return "", "", "", fmt.Errorf("invalid time: hour 0-23, minute 0-59")
		}
		return "daily", hourMin, message, nil
	}
	days, err := parseWeekdayPrefix(prefix)
	if err != nil {
		return "", "", "", err
	}
	// Encode: sorted unique weekday ints, then time
	var b strings.Builder
	for i, d := range days {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(strconv.Itoa(d))
	}
	b.WriteString(",")
	b.WriteString(hourMin)
	return "weekly", b.String(), message, nil
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

// parseWeeklyScheduleParam splits schedule_param into weekday ints and "HH:mm" time part.
func parseWeeklyScheduleParam(param string) (days []int, timePart string, ok bool) {
	parts := strings.Split(param, ",")
	if len(parts) < 2 {
		return nil, "", false
	}
	timePart = parts[len(parts)-1]
	if len(timePart) != 5 || timePart[2] != ':' {
		return nil, "", false
	}
	for _, p := range parts[:len(parts)-1] {
		w, err := strconv.Atoi(p)
		if err != nil {
			return nil, "", false
		}
		if w < 0 || w > 6 {
			return nil, "", false
		}
		days = append(days, w)
	}
	return days, timePart, true
}

func weekdaySetContains(days []int, w int) bool {
	for _, d := range days {
		if d == w {
			return true
		}
	}
	return false
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
			days, tpart, ok := parseWeeklyScheduleParam(r.ScheduleParam)
			if ok && tpart == currentHMM && weekdaySetContains(days, weekday) {
				due = append(due, r)
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
