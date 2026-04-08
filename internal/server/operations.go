package server

import (
	"errors"
	"sync"
	"time"
)

var ErrOperationInProgress = errors.New("another operation is in progress")

const (
	operationStatusRunning   = "running"
	operationStatusSucceeded = "succeeded"
	operationStatusFailed    = "failed"
	operationStatusRejected  = "rejected"
	defaultOperationLogLimit = 200
)

type operationEntry struct {
	ID         uint64
	Type       string
	Status     string
	Message    string
	StartedAt  time.Time
	FinishedAt *time.Time
}

type operationManager struct {
	mu         sync.Mutex
	now        func() time.Time
	entries    []operationEntry
	maxEntries int
	nextID     uint64
	running    bool
}

func newOperationManager(now func() time.Time, maxEntries int) *operationManager {
	if now == nil {
		now = func() time.Time {
			return time.Now().UTC()
		}
	}

	if maxEntries < 1 {
		maxEntries = defaultOperationLogLimit
	}

	return &operationManager{
		now:        now,
		maxEntries: maxEntries,
	}
}

func (m *operationManager) Run(operationType string, fn func() error) error {
	operationID, err := m.start(operationType)
	if err != nil {
		m.reject(operationType, err.Error())
		return err
	}

	err = fn()
	if err != nil {
		m.finish(operationID, operationStatusFailed, err.Error())
		return err
	}

	m.finish(operationID, operationStatusSucceeded, "")
	return nil
}

func (m *operationManager) List(limit int) []operationEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	if limit < 1 || limit > len(m.entries) {
		limit = len(m.entries)
	}

	start := len(m.entries) - limit
	cloned := make([]operationEntry, 0, limit)
	for _, entry := range m.entries[start:] {
		copyEntry := entry
		if entry.FinishedAt != nil {
			finishedAt := *entry.FinishedAt
			copyEntry.FinishedAt = &finishedAt
		}
		cloned = append(cloned, copyEntry)
	}

	return cloned
}

func (m *operationManager) start(operationType string) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return 0, ErrOperationInProgress
	}

	m.nextID++
	now := m.now().UTC()
	m.entries = append(m.entries, operationEntry{
		ID:        m.nextID,
		Type:      operationType,
		Status:    operationStatusRunning,
		StartedAt: now,
	})
	m.trimEntriesLocked()
	m.running = true

	return m.nextID, nil
}

func (m *operationManager) reject(operationType string, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	now := m.now().UTC()
	m.entries = append(m.entries, operationEntry{
		ID:         m.nextID,
		Type:       operationType,
		Status:     operationStatusRejected,
		Message:    message,
		StartedAt:  now,
		FinishedAt: &now,
	})
	m.trimEntriesLocked()
}

func (m *operationManager) finish(operationID uint64, status string, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now().UTC()
	for i := range m.entries {
		if m.entries[i].ID != operationID {
			continue
		}

		m.entries[i].Status = status
		m.entries[i].Message = message
		m.entries[i].FinishedAt = &now
		break
	}

	m.running = false
}

func (m *operationManager) trimEntriesLocked() {
	if len(m.entries) <= m.maxEntries {
		return
	}

	start := len(m.entries) - m.maxEntries
	m.entries = append([]operationEntry(nil), m.entries[start:]...)
}
