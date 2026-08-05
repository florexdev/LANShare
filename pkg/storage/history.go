package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type TransferRecord struct {
	ID           string    `json:"id"`
	FileName     string    `json:"file_name"`
	FilePath     string    `json:"file_path,omitempty"`
	FileSize     int64     `json:"file_size"`
	IsFolder     bool      `json:"is_folder"`
	Direction    string    `json:"direction"` // "incoming" or "outgoing"
	PeerID       string    `json:"peer_id"`
	PeerName     string    `json:"peer_name"`
	PeerOS       string    `json:"peer_os"`
	Status       string    `json:"status"` // "completed", "interrupted", "failed", "cancelled"
	AverageSpeed float64   `json:"average_speed"` // in MB/s
	E2EE         bool      `json:"e2ee"`
	Timestamp    time.Time `json:"timestamp"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

type HistoryStore struct {
	records  []*TransferRecord
	filePath string
	mu       sync.RWMutex
}

func NewHistoryStore() (*HistoryStore, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	appDir := filepath.Join(homeDir, ".lanshare")
	_ = os.MkdirAll(appDir, 0755)

	filePath := filepath.Join(appDir, "history.json")

	store := &HistoryStore{
		records:  make([]*TransferRecord, 0),
		filePath: filePath,
	}

	_ = store.load()
	return store, nil
}

func (h *HistoryStore) load() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := os.ReadFile(h.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &h.records)
}

func (h *HistoryStore) save() error {
	data, err := json.MarshalIndent(h.records, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(h.filePath, data, 0644)
}

func (h *HistoryStore) AddRecord(rec *TransferRecord) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now()
	}

	// Prepend to show newest first
	h.records = append([]*TransferRecord{rec}, h.records...)

	// Limit history size to 500 entries
	if len(h.records) > 500 {
		h.records = h.records[:500]
	}

	return h.save()
}

func (h *HistoryStore) UpdateRecordStatus(id string, status string, avgSpeed float64, errStr string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, rec := range h.records {
		if rec.ID == id {
			rec.Status = status
			if avgSpeed > 0 {
				rec.AverageSpeed = avgSpeed
			}
			if errStr != "" {
				rec.ErrorMessage = errStr
			}
			_ = h.save()
			break
		}
	}
}

func (h *HistoryStore) GetRecords() []*TransferRecord {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]*TransferRecord, len(h.records))
	copy(result, h.records)
	return result
}

func (h *HistoryStore) ClearHistory() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.records = make([]*TransferRecord, 0)
	return h.save()
}
