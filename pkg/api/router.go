package api

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"lanshare/pkg/config"
	"lanshare/pkg/discovery"
	"lanshare/pkg/storage"
	"lanshare/pkg/transfer"
)

type Router struct {
	cfg       *config.Config
	discovery *discovery.DiscoveryService
	transferS *transfer.TransferServer
	transferC *transfer.TransferClient
	history   *storage.HistoryStore
	hub       *WSHub
}

func NewRouter(cfg *config.Config, ds *discovery.DiscoveryService, ts *transfer.TransferServer, tc *transfer.TransferClient, hs *storage.HistoryStore, hub *WSHub) *Router {
	return &Router{
		cfg:       cfg,
		discovery: ds,
		transferS: ts,
		transferC: tc,
		history:   hs,
		hub:       hub,
	}
}

func (r *Router) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/peers", r.handlePeers)
	mux.HandleFunc("/api/beacon", r.handleBeacon)
	mux.HandleFunc("/api/config", r.handleConfig)
	mux.HandleFunc("/api/history", r.handleHistory)
	mux.HandleFunc("/api/send", r.handleSend)
	mux.HandleFunc("/api/send_web", r.handleSendWeb)
	mux.HandleFunc("/api/action", r.handleAction)
	mux.HandleFunc("/api/open_folder", r.handleOpenFolder)
	mux.HandleFunc("/ws", r.handleWS)
}

func (r *Router) handleBeacon(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost {
		var payload discovery.BeaconPayload
		if err := json.NewDecoder(req.Body).Decode(&payload); err == nil {
			if payload.ID != "" && payload.ID != r.cfg.DeviceID {
				r.discovery.HandlePeerBeaconExternal(payload, "127.0.0.1")
			}
		}
	}

	myPayload := discovery.BeaconPayload{
		ID:        r.cfg.DeviceID,
		Name:      r.cfg.DeviceName,
		OS:        r.cfg.OS,
		Port:      r.cfg.UIPort,
		E2EE:      r.cfg.E2EEEnabled,
		PublicKey: r.cfg.SecretKey,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(myPayload)
}

func (r *Router) handlePeers(w http.ResponseWriter, req *http.Request) {
	peers := r.discovery.GetActivePeers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

func (r *Router) handleConfig(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if req.Method == http.MethodGet {
		json.NewEncoder(w).Encode(r.cfg)
		return
	}

	if req.Method == http.MethodPost {
		var update struct {
			DeviceName  string `json:"device_name"`
			DownloadDir string `json:"download_dir"`
			E2EEEnabled bool   `json:"e2ee_enabled"`
			AutoAccept  bool   `json:"auto_accept"`
		}
		if err := json.NewDecoder(req.Body).Decode(&update); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := r.cfg.Update(update.DeviceName, update.DownloadDir, update.E2EEEnabled, update.AutoAccept); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Broadcast config change
		r.hub.Broadcast("config_updated", r.cfg)
		json.NewEncoder(w).Encode(r.cfg)
	}
}

func (r *Router) handleHistory(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if req.Method == http.MethodDelete {
		_ = r.history.ClearHistory()
		r.hub.Broadcast("history_cleared", nil)
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
		return
	}

	records := r.history.GetRecords()
	json.NewEncoder(w).Encode(records)
}

func (r *Router) handleSend(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		PeerIP    string `json:"peer_ip"`
		PeerPort  int    `json:"peer_port"`
		PeerID    string `json:"peer_id"`
		PeerName  string `json:"peer_name"`
		PeerOS    string `json:"peer_os"`
		LocalPath string `json:"local_path"`
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	sessionID, err := r.transferC.SendPath(body.PeerIP, body.PeerPort, body.LocalPath, body.PeerID, body.PeerName, body.PeerOS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"session_id": sessionID,
		"status":     "initiated",
	})
}

func (r *Router) handleAction(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"` // "pause", "resume", "cancel", "accept", "decline"
	}

	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if body.Action == "accept" || body.Action == "decline" {
		r.transferS.AcceptSession(body.SessionID, body.Action == "accept")
	} else {
		_ = r.transferC.Action(body.SessionID, body.Action)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (r *Router) handleWS(w http.ResponseWriter, req *http.Request) {
	r.hub.HandleWebSocket(w, req, func(client *WSClient, msg []byte) {
		// Handle incoming WS client messages if needed
	})
}

func (r *Router) handleSendWeb(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form up to 64MB memory limit, spill to temp disk
	if err := req.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "Failed to parse upload form: "+err.Error(), http.StatusBadRequest)
		return
	}

	peerIP := req.FormValue("peer_ip")
	peerPortStr := req.FormValue("peer_port")
	peerID := req.FormValue("peer_id")
	peerName := req.FormValue("peer_name")
	peerOS := req.FormValue("peer_os")
	fileName := req.FormValue("file_name")

	peerPort, _ := strconv.Atoi(peerPortStr)

	file, header, err := req.FormFile("file")
	if err != nil {
		http.Error(w, "Missing file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	if fileName == "" && header != nil {
		fileName = header.Filename
	}

	var fileSize int64 = 0
	if header != nil {
		fileSize = header.Size
	}

	sessionID, err := r.transferC.SendStream(peerIP, peerPort, file, fileSize, fileName, peerID, peerName, peerOS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"session_id": sessionID,
		"status":     "initiated",
	})
}

func (r *Router) handleOpenFolder(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Path string `json:"path"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body)

	targetPath := body.Path
	if targetPath == "" {
		targetPath = r.cfg.DownloadDir
	}

	// If targetPath is a file, get parent dir
	if fi, err := os.Stat(targetPath); err == nil && !fi.IsDir() {
		targetPath = r.cfg.DownloadDir
	}

	_ = os.MkdirAll(targetPath, 0755)

	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("explorer", targetPath).Start()
	case "darwin":
		err = exec.Command("open", targetPath).Start()
	case "linux":
		err = exec.Command("xdg-open", targetPath).Start()
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
