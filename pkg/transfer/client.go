package transfer

import (
	"archive/tar"
	"bytes"
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

type ClientSession struct {
	SessionID    string
	TargetIP     string
	TargetPort   int
	Manifest     *TransferManifest
	LocalPath    string
	Status       string // "preparing", "waiting", "transferring", "paused", "completed", "failed", "cancelled"
	BytesSent    int64
	TotalBytes   int64
	StartTime    time.Time
	LastUpdateTime time.Time
	LastBytes    int64
	CurrentSpeed float64
	cancelChan   chan struct{}
	pauseChan    chan struct{}
	resumeChan   chan struct{}
	mu           sync.RWMutex
}

type TransferClient struct {
	cfg        *config.Config
	history    *storage.HistoryStore
	httpClient *http.Client
	sessions   map[string]*ClientSession
	mu         sync.RWMutex
	onProgress func(prog *TransferProgress)
}

func NewTransferClient(cfg *config.Config, history *storage.HistoryStore, onProgress func(*TransferProgress)) *TransferClient {
	return &TransferClient{
		cfg:        cfg,
		history:    history,
		httpClient: &http.Client{Timeout: 0}, // No global timeout for long file streams
		sessions:   make(map[string]*ClientSession),
		onProgress: onProgress,
	}
}

func (tc *TransferClient) SendPath(targetIP string, targetPort int, localPath string, peerID string, peerName string, peerOS string) (string, error) {
	fi, err := os.Stat(localPath)
	if err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}

	sessionID := fmt.Sprintf("tx-%d", time.Now().UnixNano())
	isFolder := fi.IsDir()
	rootName := filepath.Base(localPath)

	var files []FileItem
	var totalSize int64 = 0

	if isFolder {
		err = filepath.Walk(localPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relPath, _ := filepath.Rel(localPath, path)
			if relPath == "." {
				return nil
			}

			files = append(files, FileItem{
				RelPath:  relPath,
				Size:     info.Size(),
				ModTime:  info.ModTime().Unix(),
				IsFolder: info.IsDir(),
			})
			if !info.IsDir() {
				totalSize += info.Size()
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("failed to scan folder: %w", err)
		}
	} else {
		totalSize = fi.Size()
		files = append(files, FileItem{
			RelPath:  rootName,
			Size:     fi.Size(),
			ModTime:  fi.ModTime().Unix(),
			IsFolder: false,
		})
	}

	manifest := &TransferManifest{
		SessionID:  sessionID,
		SenderID:   tc.cfg.DeviceID,
		SenderName: tc.cfg.DeviceName,
		SenderOS:   tc.cfg.OS,
		TotalFiles: len(files),
		TotalSize:  totalSize,
		IsFolder:   isFolder,
		RootName:   rootName,
		Files:      files,
		E2EE:       tc.cfg.E2EEEnabled,
	}

	sess := &ClientSession{
		SessionID:  sessionID,
		TargetIP:   targetIP,
		TargetPort: targetPort,
		Manifest:   manifest,
		LocalPath:  localPath,
		Status:     "preparing",
		TotalBytes: totalSize,
		cancelChan: make(chan struct{}),
		pauseChan:  make(chan struct{}),
		resumeChan: make(chan struct{}),
	}

	tc.mu.Lock()
	tc.sessions[sessionID] = sess
	tc.mu.Unlock()

	// History record placeholder
	rec := &storage.TransferRecord{
		ID:        sessionID,
		FileName:  rootName,
		FilePath:  localPath,
		FileSize:  totalSize,
		IsFolder:  isFolder,
		Direction: "outgoing",
		PeerID:    peerID,
		PeerName:  peerName,
		PeerOS:    peerOS,
		Status:    "pending",
		E2EE:      tc.cfg.E2EEEnabled,
		Timestamp: time.Now(),
	}
	_ = tc.history.AddRecord(rec)

	go tc.executeTransfer(sess)

	return sessionID, nil
}

func (tc *TransferClient) SendStream(targetIP string, targetPort int, reader io.Reader, fileSize int64, fileName string, peerID string, peerName string, peerOS string) (string, error) {
	sessionID := fmt.Sprintf("tx-%d", time.Now().UnixNano())

	manifest := &TransferManifest{
		SessionID:  sessionID,
		SenderID:   tc.cfg.DeviceID,
		SenderName: tc.cfg.DeviceName,
		SenderOS:   tc.cfg.OS,
		TotalFiles: 1,
		TotalSize:  fileSize,
		IsFolder:   false,
		RootName:   fileName,
		Files: []FileItem{
			{
				RelPath:  fileName,
				Size:     fileSize,
				ModTime:  time.Now().Unix(),
				IsFolder: false,
			},
		},
		E2EE: tc.cfg.E2EEEnabled,
	}

	sess := &ClientSession{
		SessionID:  sessionID,
		TargetIP:   targetIP,
		TargetPort: targetPort,
		Manifest:   manifest,
		Status:     "preparing",
		TotalBytes: fileSize,
		cancelChan: make(chan struct{}),
		pauseChan:  make(chan struct{}),
		resumeChan: make(chan struct{}),
	}

	tc.mu.Lock()
	tc.sessions[sessionID] = sess
	tc.mu.Unlock()

	rec := &storage.TransferRecord{
		ID:        sessionID,
		FileName:  fileName,
		FileSize:  fileSize,
		IsFolder:  false,
		Direction: "outgoing",
		PeerID:    peerID,
		PeerName:  peerName,
		PeerOS:    peerOS,
		Status:    "pending",
		E2EE:      tc.cfg.E2EEEnabled,
		Timestamp: time.Now(),
	}
	_ = tc.history.AddRecord(rec)

	go tc.executeStreamTransfer(sess, reader)

	return sessionID, nil
}

func (tc *TransferClient) executeStreamTransfer(sess *ClientSession, reader io.Reader) {
	// 1. Prepare Request to remote peer
	sess.mu.Lock()
	sess.Status = "waiting"
	sess.mu.Unlock()
	tc.emitProgress(sess, "waiting", "Waiting for recipient to accept...")

	prepURL := fmt.Sprintf("http://%s:%d/api/transfer/prepare", sess.TargetIP, sess.TargetPort)
	manifestData, _ := json.Marshal(sess.Manifest)

	req, err := http.NewRequest(http.MethodPost, prepURL, bytes.NewBuffer(manifestData))
	if err != nil {
		tc.failSession(sess, fmt.Errorf("failed to build prepare request: %w", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		tc.failSession(sess, fmt.Errorf("connection to remote peer failed: %w", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tc.failSession(sess, fmt.Errorf("remote peer rejected prepare request: status %d", resp.StatusCode))
		return
	}

	var prepResp struct {
		Accepted bool   `json:"accepted"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepResp); err != nil || !prepResp.Accepted {
		reason := prepResp.Reason
		if reason == "" {
			reason = "Transfer declined by recipient"
		}
		tc.failSession(sess, fmt.Errorf("%s", reason))
		return
	}

	// 2. Peer Accepted -> Stream file upload
	sess.mu.Lock()
	sess.Status = "transferring"
	sess.StartTime = time.Now()
	sess.LastUpdateTime = time.Now()
	sess.mu.Unlock()

	tc.emitProgress(sess, "transferring", sess.Manifest.RootName)

	uploadURL := fmt.Sprintf("http://%s:%d/api/transfer/upload", sess.TargetIP, sess.TargetPort)

	// Wrap reader with progress counter
	pw := &ProgressReader{
		Reader: reader,
		OnRead: func(n int) {
			sess.mu.Lock()
			sess.BytesSent += int64(n)
			tc.updateSpeedMetrics(sess)
			sess.mu.Unlock()

			tc.emitProgress(sess, "transferring", sess.Manifest.RootName)
		},
		CheckPauseCancel: func() string {
			sess.mu.RLock()
			st := sess.Status
			sess.mu.RUnlock()
			return st
		},
	}

	upReq, err := http.NewRequest(http.MethodPost, uploadURL, pw)
	if err != nil {
		tc.failSession(sess, err)
		return
	}
	upReq.Header.Set("X-Session-ID", sess.SessionID)

	upResp, err := tc.httpClient.Do(upReq)
	if err != nil {
		tc.failSession(sess, fmt.Errorf("upload stream interrupted: %w", err))
		return
	}
	defer upResp.Body.Close()

	if upResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(upResp.Body)
		tc.failSession(sess, fmt.Errorf("upload failed: %s", string(bodyBytes)))
		return
	}

	sess.mu.Lock()
	sess.Status = "completed"
	sess.BytesSent = sess.TotalBytes
	sess.mu.Unlock()

	tc.history.UpdateRecordStatus(sess.SessionID, "completed", sess.CurrentSpeed, "")
	tc.emitProgress(sess, "completed", "")
}

func (tc *TransferClient) executeTransfer(sess *ClientSession) {
	// 1. Prepare Request to remote peer
	sess.mu.Lock()
	sess.Status = "waiting"
	sess.mu.Unlock()
	tc.emitProgress(sess, "waiting", "Waiting for recipient to accept...")

	prepURL := fmt.Sprintf("http://%s:%d/api/transfer/prepare", sess.TargetIP, sess.TargetPort)
	manifestData, _ := json.Marshal(sess.Manifest)

	req, err := http.NewRequest(http.MethodPost, prepURL, bytes.NewBuffer(manifestData))
	if err != nil {
		tc.failSession(sess, fmt.Errorf("failed to build prepare request: %w", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		tc.failSession(sess, fmt.Errorf("connection to remote peer failed: %w", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tc.failSession(sess, fmt.Errorf("remote peer rejected prepare request: status %d", resp.StatusCode))
		return
	}

	var prepResp struct {
		Accepted bool   `json:"accepted"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prepResp); err != nil || !prepResp.Accepted {
		reason := prepResp.Reason
		if reason == "" {
			reason = "Transfer declined by recipient"
		}
		tc.failSession(sess, fmt.Errorf("%s", reason))
		return
	}

	// 2. Peer Accepted -> Stream file upload
	sess.mu.Lock()
	sess.Status = "transferring"
	sess.StartTime = time.Now()
	sess.LastUpdateTime = time.Now()
	sess.mu.Unlock()

	tc.emitProgress(sess, "transferring", sess.Manifest.RootName)

	uploadURL := fmt.Sprintf("http://%s:%d/api/transfer/upload", sess.TargetIP, sess.TargetPort)

	var bodyReader io.Reader
	var resumeOffset int64 = 0

	if sess.Manifest.IsFolder {
		bodyReader = tc.createTarStream(sess)
	} else {
		f, err := os.Open(sess.LocalPath)
		if err != nil {
			tc.failSession(sess, fmt.Errorf("failed to open local file: %w", err))
			return
		}
		defer f.Close()
		bodyReader = f
	}

	// Wrap reader with progress counter
	pw := &ProgressReader{
		Reader: bodyReader,
		OnRead: func(n int) {
			sess.mu.Lock()
			sess.BytesSent += int64(n)
			tc.updateSpeedMetrics(sess)
			sess.mu.Unlock()

			tc.emitProgress(sess, "transferring", sess.Manifest.RootName)
		},
		CheckPauseCancel: func() string {
			sess.mu.RLock()
			st := sess.Status
			sess.mu.RUnlock()
			return st
		},
	}

	upReq, err := http.NewRequest(http.MethodPost, uploadURL, pw)
	if err != nil {
		tc.failSession(sess, err)
		return
	}
	upReq.Header.Set("X-Session-ID", sess.SessionID)
	if resumeOffset > 0 {
		upReq.Header.Set("X-Resume-Offset", strconv.FormatInt(resumeOffset, 10))
	}

	upResp, err := tc.httpClient.Do(upReq)
	if err != nil {
		tc.failSession(sess, fmt.Errorf("upload stream interrupted: %w", err))
		return
	}
	defer upResp.Body.Close()

	if upResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(upResp.Body)
		tc.failSession(sess, fmt.Errorf("upload failed: %s", string(bodyBytes)))
		return
	}

	sess.mu.Lock()
	sess.Status = "completed"
	sess.BytesSent = sess.TotalBytes
	sess.mu.Unlock()

	tc.history.UpdateRecordStatus(sess.SessionID, "completed", sess.CurrentSpeed, "")
	tc.emitProgress(sess, "completed", "")
}

func (tc *TransferClient) createTarStream(sess *ClientSession) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		gzw := gzip.NewWriter(pw)
		tw := tar.NewWriter(gzw)

		err := filepath.Walk(sess.LocalPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			relPath, err := filepath.Rel(filepath.Dir(sess.LocalPath), path)
			if err != nil {
				return err
			}

			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(relPath)

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if !info.IsDir() {
				file, err := os.Open(path)
				if err != nil {
					return err
				}
				defer file.Close()

				bufPtr := BufferPool.Get().(*[]byte)
				defer BufferPool.Put(bufPtr)
				buf := *bufPtr

				if _, err := io.CopyBuffer(tw, file, buf); err != nil {
					return err
				}
			}
			return nil
		})

		_ = tw.Close()
		_ = gzw.Close()

		if err != nil {
			_ = pw.CloseWithError(err)
		} else {
			_ = pw.Close()
		}
	}()

	return pr
}

func (tc *TransferClient) failSession(sess *ClientSession, err error) {
	log.Printf("[!] Transfer failed for session %s: %v\n", sess.SessionID, err)
	sess.mu.Lock()
	sess.Status = "failed"
	sess.mu.Unlock()

	tc.history.UpdateRecordStatus(sess.SessionID, "failed", sess.CurrentSpeed, err.Error())
	tc.emitProgress(sess, "failed", err.Error())
}

func (tc *TransferClient) Action(sessionID string, action string) error {
	tc.mu.RLock()
	sess, ok := tc.sessions[sessionID]
	tc.mu.RUnlock()

	if !ok || sess == nil {
		return fmt.Errorf("session not found")
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	switch action {
	case "pause":
		sess.Status = "paused"
		tc.history.UpdateRecordStatus(sessionID, "paused", sess.CurrentSpeed, "")
	case "resume":
		sess.Status = "transferring"
	case "cancel":
		sess.Status = "cancelled"
		tc.history.UpdateRecordStatus(sessionID, "cancelled", sess.CurrentSpeed, "User cancelled transfer")
	}

	tc.emitProgress(sess, action, "")
	return nil
}

func (tc *TransferClient) updateSpeedMetrics(sess *ClientSession) {
	now := time.Now()
	elapsed := now.Sub(sess.LastUpdateTime).Seconds()
	if elapsed >= 0.5 {
		bytesDiff := sess.BytesSent - sess.LastBytes
		speedMBs := (float64(bytesDiff) / (1024 * 1024)) / elapsed
		sess.CurrentSpeed = speedMBs
		sess.LastBytes = sess.BytesSent
		sess.LastUpdateTime = now
	}
}

func (tc *TransferClient) emitProgress(sess *ClientSession, status string, currentFile string) {
	if tc.onProgress == nil {
		return
	}

	sess.mu.RLock()
	pct := 0.0
	if sess.TotalBytes > 0 {
		pct = (float64(sess.BytesSent) / float64(sess.TotalBytes)) * 100.0
		if pct > 100.0 {
			pct = 100.0
		}
	}

	var eta int64 = 0
	if sess.CurrentSpeed > 0 {
		remainingMB := float64(sess.TotalBytes-sess.BytesSent) / (1024 * 1024)
		eta = int64(remainingMB / sess.CurrentSpeed)
		if eta < 0 {
			eta = 0
		}
	}

	prog := &TransferProgress{
		SessionID:    sess.SessionID,
		Status:       status,
		BytesSent:    sess.BytesSent,
		TotalBytes:   sess.TotalBytes,
		ProgressPct:  pct,
		CurrentSpeed: sess.CurrentSpeed,
		ETASeconds:   eta,
		CurrentFile:  currentFile,
	}
	sess.mu.RUnlock()

	tc.onProgress(prog)
}

type ProgressReader struct {
	Reader           io.Reader
	OnRead           func(n int)
	CheckPauseCancel func() string
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	if pr.CheckPauseCancel != nil {
		status := pr.CheckPauseCancel()
		if status == "paused" {
			time.Sleep(200 * time.Millisecond)
			return 0, nil
		}
		if status == "cancelled" {
			return 0, fmt.Errorf("transfer cancelled by user")
		}
	}

	n, err := pr.Reader.Read(p)
	if n > 0 && pr.OnRead != nil {
		pr.OnRead(n)
	}
	return n, err
}
