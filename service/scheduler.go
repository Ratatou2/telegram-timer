package service

import (
	"log"
	"strconv"
	"sync"
	"time"
)

// Sender sends a message to a chat (e.g. Telegram client).
type Sender interface {
	SendMessage(chatID int64, text string) error
}

// Scheduler runs every 1 minute, fetches due reminders, sends them and marks sent.
type Scheduler struct {
	reminder *ReminderService
	sender   Sender
	interval time.Duration
	mu       sync.Mutex
	stop     chan struct{}
}

// NewScheduler returns a Scheduler with 1-minute interval.
func NewScheduler(reminder *ReminderService, sender Sender) *Scheduler {
	return &Scheduler{
		reminder: reminder,
		sender:   sender,
		interval: time.Minute,
		stop:     make(chan struct{}),
	}
}

// Start runs the ticker loop. It blocks until Stop is called.
func (s *Scheduler) Start() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.runOnce()
		}
	}
}

// runOnce processes due reminders once. Only one run at a time.
// Sends advance notifications (30m, 10m, 5m before) then on-time notifications.
func (s *Scheduler) runOnce() {
	if !s.mu.TryLock() {
		return
	}
	defer s.mu.Unlock()

	now := time.Now()
	if s.reminder.nowFunc != nil {
		now = s.reminder.nowFunc()
	}
	// Advance notifications: 30m, 10m, 5m before
	for _, advanceMin := range []int{30, 10, 5} {
		list, err := s.reminder.ListDueAdvance(now, advanceMin)
		if err != nil {
			log.Printf("scheduler ListDueAdvance(%dm): %v", advanceMin, err)
			continue
		}
		label := strconv.Itoa(advanceMin) + "분 전"
		for _, r := range list {
			text := "⏰ [" + label + "] " + r.RemindAt.Format("15:04") + " " + r.Message
			if err := s.sender.SendMessage(r.ChatID, text); err != nil {
				log.Printf("scheduler SendMessage chat=%d: %v", r.ChatID, err)
				continue
			}
			var markErr error
			switch advanceMin {
			case 30:
				markErr = s.reminder.MarkSent30m(r.Id)
			case 10:
				markErr = s.reminder.MarkSent10m(r.Id)
			case 5:
				markErr = s.reminder.MarkSent5m(r.Id)
			}
			if markErr != nil {
				log.Printf("scheduler MarkSent%dm id=%d: %v", advanceMin, r.Id, markErr)
			}
		}
	}
	// On-time notifications
	list, err := s.reminder.ListDue(now)
	if err != nil {
		log.Printf("scheduler ListDue: %v", err)
		return
	}
	for _, r := range list {
		text := "⏰ " + r.RemindAt.Format("15:04") + " " + r.Message
		if err := s.sender.SendMessage(r.ChatID, text); err != nil {
			log.Printf("scheduler SendMessage chat=%d: %v", r.ChatID, err)
			continue
		}
		if err := s.reminder.MarkSent(r.Id, now); err != nil {
			log.Printf("scheduler MarkSent id=%d: %v", r.Id, err)
		}
	}
}

// Stop stops the scheduler loop.
func (s *Scheduler) Stop() {
	close(s.stop)
}
