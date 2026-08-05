package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"lanshare/pkg/api"
	"lanshare/pkg/config"
	"lanshare/pkg/discovery"
	"lanshare/pkg/storage"
	"lanshare/pkg/transfer"
)

//go:embed all:web
var webAssets embed.FS

func main() {
	flagName := flag.String("name", "", "Custom Device Name for multi-instance testing")
	flagPort := flag.Int("port", 0, "Custom UI Port for multi-instance testing")
	flagTPort := flag.Int("tport", 0, "Custom Transfer Port for multi-instance testing")
	flagDir := flag.String("dir", "", "Custom Download Directory for multi-instance testing")
	flagID := flag.String("id", "", "Custom Device ID for multi-instance testing")
	flag.Parse()

	fmt.Println("=====================================================")
	fmt.Println(" LANShare - High Speed Cross-Platform LAN Sharing   ")
	fmt.Println("=====================================================")

	// 1. Initialize Configuration
	cfg, err := config.DefaultConfig()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	if *flagName != "" {
		cfg.DeviceName = *flagName
	}
	if *flagPort != 0 {
		cfg.UIPort = *flagPort
	}
	if *flagTPort != 0 {
		cfg.TransferPort = *flagTPort
	}
	if *flagDir != "" {
		cfg.DownloadDir = *flagDir
		_ = os.MkdirAll(*flagDir, 0755)
	}
	if *flagID != "" {
		cfg.DeviceID = *flagID
	}

	fmt.Printf("[+] Device Name: %s (%s)\n", cfg.DeviceName, cfg.OS)
	fmt.Printf("[+] Download Directory: %s\n", cfg.DownloadDir)

	// 2. Initialize Storage & History
	history, err := storage.NewHistoryStore()
	if err != nil {
		log.Fatalf("Failed to initialize history store: %v", err)
	}

	// 3. Initialize WebSocket Hub
	hub := api.NewWSHub()
	go hub.Run()

	// 4. Initialize Discovery Service
	ds := discovery.NewDiscoveryService(cfg)
	if err := ds.Start(); err != nil {
		log.Printf("[!] Warning: UDP Discovery listener warning: %v\n", err)
	} else {
		fmt.Printf("[+] Peer Discovery active on UDP port %d\n", cfg.DiscoveryPort)
	}
	defer ds.Stop()

	// Pipe discovery events to WebSocket subscribers
	go func() {
		for evt := range ds.Events() {
			hub.Broadcast("peer_event", evt)
		}
	}()

	// 5. Initialize Transfer Engine
	onPrompt := func(manifest *transfer.TransferManifest) {
		hub.Broadcast("transfer_prompt", manifest)
	}
	onProgress := func(prog *transfer.TransferProgress) {
		hub.Broadcast("transfer_progress", prog)
	}

	transferServer := transfer.NewTransferServer(cfg, history, onPrompt, onProgress)
	transferClient := transfer.NewTransferClient(cfg, history, onProgress)

	// 6. Setup HTTP Serve Mux & Embed Web Assets
	mux := http.NewServeMux()

	// Transfer API handlers
	transferServer.RegisterHandlers(mux)

	// Main App Router API & WebSocket handlers
	appRouter := api.NewRouter(cfg, ds, transferServer, transferClient, history, hub)
	appRouter.Register(mux)

	// Serve embedded web frontend assets
	webSubFS, err := fs.Sub(webAssets, "web")
	if err != nil {
		log.Fatalf("Failed to load embedded web assets: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(webSubFS)))

	// 7. Start HTTP Server
	serverAddr := fmt.Sprintf("0.0.0.0:%d", cfg.UIPort)
	localURL := fmt.Sprintf("http://localhost:%d", cfg.UIPort)

	fmt.Printf("[+] Web UI & API server running on %s\n", localURL)

	// Launch default web browser automatically
	go func() {
		time.Sleep(500 * time.Millisecond)
		openBrowser(localURL)
	}()

	log.Printf("[+] LANShare ready! Press Ctrl+C to stop.\n")
	if err := http.ListenAndServe(serverAddr, mux); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	}
	if err != nil {
		log.Printf("Could not auto-open browser: %v. Please navigate to %s", err, url)
	}
}
