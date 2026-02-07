package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"telegram-timer/service"
	"telegram-timer/telegram"
)

// Update is the Telegram webhook payload.
type Update struct {
	Message *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

// Telegram handles POST /telegram/webhook: parse Update, route command or register reminder, send response.
type Telegram struct {
	reminder *service.ReminderService
	sender   *telegram.Client
}

// NewTelegram returns a Telegram handler.
func NewTelegram(reminder *service.ReminderService, sender *telegram.Client) *Telegram {
	return &Telegram{reminder: reminder, sender: sender}
}

// ServeHTTP handles Telegram webhook POST.
func (h *Telegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var u Update
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		log.Printf("telegram webhook decode: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if u.Message == nil || u.Message.Text == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	chatID := u.Message.Chat.ID
	text := strings.TrimSpace(u.Message.Text)
	now := time.Now()

	var reply string
	switch {
	case text == "/list":
		reply = h.handleList(chatID, now)
	case strings.HasPrefix(text, "/delete "):
		reply = h.handleDelete(chatID, text, now)
	case text == "/delete":
		reply = "사용법: /delete {번호}"
	default:
		reply = h.handleRegister(chatID, text, now)
	}

	if reply != "" {
		if err := h.sender.SendMessage(chatID, reply); err != nil {
			log.Printf("send message: %v", err)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Telegram) handleList(chatID int64, now time.Time) string {
	list, err := h.reminder.ListUnsent(chatID, now)
	if err != nil {
		log.Printf("list unsent: %v", err)
		return "목록 조회 중 오류가 발생했습니다."
	}
	if len(list) == 0 {
		return "등록된 알림이 없습니다."
	}
	var b strings.Builder
	for i, r := range list {
		t := r.RemindAt.Format("15:04")
		b.WriteString("(" +strconv.Itoa(i+1) + ") " + t + " " + r.Message + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func (h *Telegram) handleDelete(chatID int64, text string, now time.Time) string {
	parts := strings.Fields(text)
	if len(parts) != 2 {
		return "사용법: /delete {번호}"
	}
	idx, err := strconv.Atoi(parts[1])
	if err != nil || idx < 1 {
		return "번호는 1 이상의 숫자여야 합니다."
	}
	if err := h.reminder.DeleteByListIndex(chatID, idx, now); err != nil {
		return err.Error()
	}
	return "삭제했습니다."
}

func (h *Telegram) handleRegister(chatID int64, text string, now time.Time) string {
	id, err := h.reminder.Create(chatID, text, now)
	if err != nil {
		return "등록 실패: " + err.Error()
	}
	list, _ := h.reminder.ListUnsent(chatID, now)
	var pos int
	var ts string
	for i, r := range list {
		if r.Id == id {
			pos = i + 1
			ts = r.RemindAt.Format("15:04")
			break
		}
	}
	return "알림을 등록했습니다. (" + ts + ", #" + strconv.Itoa(pos) + ")"
}
