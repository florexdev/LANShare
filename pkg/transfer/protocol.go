package transfer

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
)

const (
	ChunkSize = 64 * 1024 // 64KB buffer for high-throughput zero-copy-like streaming
)

// Buffer pool to reuse memory allocations during file streaming
var BufferPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, ChunkSize)
		return &b
	},
}

type FileItem struct {
	RelPath  string `json:"rel_path"`  // Relative path inside folder or filename
	Size     int64  `json:"size"`      // Size in bytes
	ModTime  int64  `json:"mod_time"`  // Unix timestamp
	IsFolder bool   `json:"is_folder"`
}

type TransferManifest struct {
	SessionID   string     `json:"session_id"`
	SenderID    string     `json:"sender_id"`
	SenderName  string     `json:"sender_name"`
	SenderOS    string     `json:"sender_os"`
	TotalFiles  int        `json:"total_files"`
	TotalSize   int64      `json:"total_size"`
	IsFolder    bool       `json:"is_folder"`
	RootName    string     `json:"root_name"`
	Files       []FileItem `json:"files"`
	E2EE        bool       `json:"e2ee"`
	CipherKey   []byte     `json:"-"` // Pre-shared or exchanged session key
}

type TransferProgress struct {
	SessionID    string  `json:"session_id"`
	Status       string  `json:"status"` // "pending", "transferring", "paused", "completed", "failed", "cancelled"
	BytesSent    int64   `json:"bytes_sent"`
	TotalBytes   int64   `json:"total_bytes"`
	ProgressPct  float64 `json:"progress_pct"`
	CurrentSpeed float64 `json:"current_speed"` // MB/s
	ETASeconds   int64   `json:"eta_seconds"`
	CurrentFile  string  `json:"current_file"`
	ErrorMessage string  `json:"error_message,omitempty"`
}

type AcceptRequest struct {
	SessionID string `json:"session_id"`
	Accept    bool   `json:"accept"`
}

type TransferAction struct {
	SessionID string `json:"session_id"`
	Action    string `json:"action"` // "pause", "resume", "cancel"
}

// AES-256-GCM Helper
func EncryptData(key []byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func DecryptData(key []byte, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
