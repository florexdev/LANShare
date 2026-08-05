package transfer

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"lanshare/pkg/config"
	"lanshare/pkg/storage"
)

type ServerSession struct {
	Manifest      *TransferManifest
	Accepted      chan bool
	Status        string // "pending", "transferring", "paused", "completed", "failed", "cancelled"
	BytesReceived int64
	TotalBytes    int64
	StartTime     time.Time
	LastUpdateTime time.Time
	LastBytes     int64
	CurrentSpeed  float64
	SavePath      string
	mu            sync.RWMutex
	CancelFunc    func()
}

type TransferServer struct {
	cfg      *config.Config
	history  *storage.HistoryStore
	sessions map[string]*ServerSession
	mu       sync.RWMutex
	onPrompt func(manifest *TransferManifest)
	onProgress func(prog *TransferProgress)
}

func NewTransferServer(cfg *config.Config, history *storage.HistoryStore, onPrompt func(*TransferManifest), onProgress func(*TransferProgress)) *TransferServer {
	return &TransferServer{
		cfg:        cfg,
		history:    history,
		sessions:   make(map[string]*ServerSession),
		onPrompt:   onPrompt,
		onProgress: onProgress,
	}
}

func (ts *TransferServer) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/transfer/prepare", ts.handlePrepare)
	mux.HandleFunc("/api/transfer/accept", ts.handleAccept)
	mux.HandleFunc("/api/transfer/upload", ts.handleUpload)
	mux.HandleFunc("/api/transfer/action", ts.handleAction)
	mux.HandleFunc("/api/transfer/status", ts.handleStatus)
}

func (ts *TransferServer) handlePrepare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var manifest TransferManifest
	if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
		http.Error(w, "Invalid manifest", http.StatusBadRequest)
		return
	}

	log.Printf("[+] Incoming file transfer request: '%s' (%d bytes) from peer '%s' (%s)\n", manifest.RootName, manifest.TotalSize, manifest.SenderName, manifest.SenderOS)

	sess := &ServerSession{
		Manifest:   &manifest,
		Accepted:   make(chan bool, 1),
		Status:     "pending",
		TotalBytes: manifest.TotalSize,
		SavePath:   filepath.Join(ts.cfg.DownloadDir, manifest.RootName),
	}

	ts.mu.Lock()
	ts.sessions[manifest.SessionID] = sess
	ts.mu.Unlock()

	// History record placeholder
	rec := &storage.TransferRecord{
		ID:        manifest.SessionID,
		FileName:  manifest.RootName,
		FilePath:  sess.SavePath,
		FileSize:  manifest.TotalSize,
		IsFolder:  manifest.IsFolder,
		Direction: "incoming",
		PeerID:    manifest.SenderID,
		PeerName:  manifest.SenderName,
		PeerOS:    manifest.SenderOS,
		Status:    "pending",
		E2EE:      manifest.E2EE,
		Timestamp: time.Now(),
	}
	_ = ts.history.AddRecord(rec)

	if ts.cfg.AutoAccept {
		sess.Accepted <- true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accepted":   true,
			"session_id": manifest.SessionID,
		})
		return
	}

	// Trigger UI confirmation prompt
	if ts.onPrompt != nil {
		ts.onPrompt(&manifest)
	}

	// Wait up to 60s for user acceptance
	select {
	case accepted := <-sess.Accepted:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accepted":   accepted,
			"session_id": manifest.SessionID,
		})
	case <-time.After(60 * time.Second):
		sess.mu.Lock()
		sess.Status = "rejected"
		sess.mu.Unlock()
		ts.history.UpdateRecordStatus(manifest.SessionID, "rejected", 0, "Transfer timed out waiting for acceptance")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accepted":   false,
			"session_id": manifest.SessionID,
			"reason":     "timeout",
		})
	}
}

func (ts *TransferServer) AcceptSession(sessionID string, accept bool) {
	ts.mu.RLock()
	sess, ok := ts.sessions[sessionID]
	ts.mu.RUnlock()

	if ok && sess != nil {
		select {
		case sess.Accepted <- accept:
		default:
		}
	}
}

func (ts *TransferServer) handleAccept(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AcceptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	ts.AcceptSession(req.SessionID, req.Accept)
	w.WriteHeader(http.StatusOK)
}

func (ts *TransferServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		http.Error(w, "Missing X-Session-ID header", http.StatusBadRequest)
		return
	}

	ts.mu.RLock()
	sess, ok := ts.sessions[sessionID]
	ts.mu.RUnlock()

	if !ok || sess == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	sess.mu.Lock()
	sess.Status = "transferring"
	sess.StartTime = time.Now()
	sess.LastUpdateTime = time.Now()
	sess.mu.Unlock()

	// Check if resumable transfer offset is specified
	resumeOffsetStr := r.Header.Get("X-Resume-Offset")
	var resumeOffset int64 = 0
	if resumeOffsetStr != "" {
		if offset, err := strconv.ParseInt(resumeOffsetStr, 10, 64); err == nil {
			resumeOffset = offset
			sess.BytesReceived = resumeOffset
		}
	}

	manifest := sess.Manifest

	var err error
	if manifest.IsFolder {
		err = ts.receiveTarArchive(r.Body, sess, resumeOffset)
	} else {
		err = ts.receiveSingleFile(r.Body, sess, resumeOffset)
	}

	sess.mu.Lock()
	if err != nil {
		if sess.Status != "paused" && sess.Status != "cancelled" {
			sess.Status = "failed"
			ts.history.UpdateRecordStatus(sessionID, "failed", sess.CurrentSpeed, err.Error())
		}
		sess.mu.Unlock()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sess.Status = "completed"
	sess.BytesReceived = sess.TotalBytes
	sess.mu.Unlock()

	ts.history.UpdateRecordStatus(sessionID, "completed", sess.CurrentSpeed, "")
	ts.emitProgress(sess, "completed", "")

	w.WriteHeader(http.StatusOK)
}

func (ts *TransferServer) receiveSingleFile(r io.Reader, sess *ServerSession, resumeOffset int64) error {
	savePath := sess.SavePath
	_ = os.MkdirAll(filepath.Dir(savePath), 0755)

	flag := os.O_CREATE | os.O_WRONLY
	if resumeOffset > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(savePath, flag, 0644)
	if err != nil {
		return fmt.Errorf("failed to open destination file: %w", err)
	}
	defer f.Close()

	if resumeOffset > 0 {
		if _, err := f.Seek(resumeOffset, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek destination file: %w", err)
		}
	}

	bufPtr := BufferPool.Get().(*[]byte)
	defer BufferPool.Put(bufPtr)
	buf := *bufPtr

	for {
		sess.mu.RLock()
		status := sess.Status
		sess.mu.RUnlock()

		if status == "paused" || status == "cancelled" {
			return fmt.Errorf("transfer %s", status)
		}

		n, readErr := r.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("failed to write to file: %w", writeErr)
			}

			sess.mu.Lock()
			sess.BytesReceived += int64(n)
			ts.updateSpeedMetrics(sess)
			sess.mu.Unlock()

			ts.emitProgress(sess, "transferring", manifestRootName(sess.Manifest))
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}

	return nil
}

func (ts *TransferServer) receiveTarArchive(r io.Reader, sess *ServerSession, resumeOffset int64) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader error: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	destDir := ts.cfg.DownloadDir

	bufPtr := BufferPool.Get().(*[]byte)
	defer BufferPool.Put(bufPtr)
	buf := *bufPtr

	for {
		sess.mu.RLock()
		status := sess.Status
		sess.mu.RUnlock()

		if status == "paused" || status == "cancelled" {
			return fmt.Errorf("transfer %s", status)
		}

		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		targetPath := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			_ = os.MkdirAll(targetPath, 0755)
		case tar.TypeReg:
			_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return err
			}

			for {
				n, readErr := tr.Read(buf)
				if n > 0 {
					_, _ = outFile.Write(buf[:n])
					sess.mu.Lock()
					sess.BytesReceived += int64(n)
					ts.updateSpeedMetrics(sess)
					sess.mu.Unlock()

					ts.emitProgress(sess, "transferring", header.Name)
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					outFile.Close()
					return readErr
				}
			}
			outFile.Close()
		}
	}

	return nil
}

func (ts *TransferServer) updateSpeedMetrics(sess *ServerSession) {
	now := time.Now()
	elapsed := now.Sub(sess.LastUpdateTime).Seconds()
	if elapsed >= 0.5 {
		bytesDiff := sess.BytesReceived - sess.LastBytes
		speedMBs := (float64(bytesDiff) / (1024 * 1024)) / elapsed
		sess.CurrentSpeed = speedMBs
		sess.LastBytes = sess.BytesReceived
		sess.LastUpdateTime = now
	}
}

func (ts *TransferServer) emitProgress(sess *ServerSession, status string, currentFile string) {
	if ts.onProgress == nil {
		return
	}

	sess.mu.RLock()
	pct := 0.0
	if sess.TotalBytes > 0 {
		pct = (float64(sess.BytesReceived) / float64(sess.TotalBytes)) * 100.0
		if pct > 100.0 {
			pct = 100.0
		}
	}

	var eta int64 = 0
	if sess.CurrentSpeed > 0 {
		remainingMB := float64(sess.TotalBytes-sess.BytesReceived) / (1024 * 1024)
		eta = int64(remainingMB / sess.CurrentSpeed)
		if eta < 0 {
			eta = 0
		}
	}

	prog := &TransferProgress{
		SessionID:    sess.Manifest.SessionID,
		Status:       status,
		BytesSent:    sess.BytesReceived,
		TotalBytes:   sess.TotalBytes,
		ProgressPct:  pct,
		CurrentSpeed: sess.CurrentSpeed,
		ETASeconds:   eta,
		CurrentFile:  currentFile,
	}
	sess.mu.RUnlock()

	ts.onProgress(prog)
}

func (ts *TransferServer) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var act TransferAction
	if err := json.NewDecoder(r.Body).Decode(&act); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	ts.mu.RLock()
	sess, ok := ts.sessions[act.SessionID]
	ts.mu.RUnlock()

	if !ok || sess == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	sess.mu.Lock()
	switch act.Action {
	case "pause":
		sess.Status = "paused"
		ts.history.UpdateRecordStatus(act.SessionID, "paused", sess.CurrentSpeed, "")
	case "resume":
		sess.Status = "transferring"
	case "cancel":
		sess.Status = "cancelled"
		ts.history.UpdateRecordStatus(act.SessionID, "cancelled", sess.CurrentSpeed, "User cancelled transfer")
	}
	sess.mu.Unlock()

	ts.emitProgress(sess, act.Action, "")
	w.WriteHeader(http.StatusOK)
}

func (ts *TransferServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	ts.mu.RLock()
	sess, ok := ts.sessions[sessionID]
	ts.mu.RUnlock()

	if !ok || sess == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	sess.mu.RLock()
	defer sess.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session_id":     sess.Manifest.SessionID,
		"bytes_received": sess.BytesReceived,
		"total_bytes":    sess.TotalBytes,
		"status":         sess.Status,
	})
}

func manifestRootName(m *TransferManifest) string {
	if m == nil {
		return ""
	}
	return m.RootName
}
