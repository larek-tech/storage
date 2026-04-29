package kafka

import (
	"sync"
	"time"
)

// RetryPolicy решает, повторять ли обработку батча и через какую задержку.
// Реализации должны быть thread-safe, если используются для нескольких consumer'ов.
type RetryPolicy interface {
	// Next вызывается после каждой неудачной обработки.
	// attempt начинается с 1.
	// Возвращает delay перед следующей попыткой и retry=false, если ретраи
	// исчерпаны (тогда Consumer передаст сообщение в DLQ или вернёт ошибку).
	Next(attempt int, err error) (delay time.Duration, retry bool)
}

// InMemoryRetryPolicy — простая retry-политика с экспоненциальным backoff.
//
// ВНИМАНИЕ: предназначена для тестов и демо. Не использовать в продакшене —
// состояние счётчика не делится между процессами, перезапуск consumer'а
// сбрасывает прогресс ретраев.
type InMemoryRetryPolicy struct {
	MaxAttempts int           // 0 → без ретраев
	BaseDelay   time.Duration // первая задержка; 0 → без задержки
	MaxDelay    time.Duration // потолок; 0 → без потолка
}

func (p InMemoryRetryPolicy) Next(attempt int, _ error) (time.Duration, bool) {
	if attempt >= p.MaxAttempts {
		return 0, false
	}
	delay := p.BaseDelay << (attempt - 1)
	if p.MaxDelay > 0 && delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	return delay, true
}

// DLQ принимает сообщения, обработка которых исчерпала ретраи.
type DLQ interface {
	Send(msg RawMessage, reason error) error
}

// InMemoryDLQ — буфер сообщений в памяти процесса.
//
// ВНИМАНИЕ: предназначен для тестов и демо. Не использовать в продакшене —
// сообщения теряются при рестарте, нет персистентности и репликации.
// В реальной системе DLQ должна быть отдельным kafka-топиком или внешним хранилищем.
type InMemoryDLQ struct {
	mu      sync.Mutex
	entries []DLQEntry
}

type DLQEntry struct {
	Message RawMessage
	Reason  error
}

func (d *InMemoryDLQ) Send(msg RawMessage, reason error) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries = append(d.entries, DLQEntry{Message: msg, Reason: reason})
	return nil
}

// Messages возвращает копию накопленных сообщений (для инспекции в тестах).
func (d *InMemoryDLQ) Messages() []DLQEntry {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]DLQEntry, len(d.entries))
	copy(out, d.entries)
	return out
}
