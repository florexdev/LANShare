package discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"lanshare/pkg/config"
)

type BeaconPayload struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	OS        string `json:"os"`
	Port      int    `json:"port"`
	E2EE      bool   `json:"e2ee"`
	PublicKey string `json:"public_key,omitempty"`
}

type DiscoveryService struct {
	cfg        *config.Config
	peers      map[string]*Peer
	events     chan PeerEvent
	mu         sync.RWMutex
	stopChan   chan struct{}
	udpConn    *net.UDPConn
	localIPs   []string
	httpClient *http.Client
}

func NewDiscoveryService(cfg *config.Config) *DiscoveryService {
	return &DiscoveryService{
		cfg:        cfg,
		peers:      make(map[string]*Peer),
		events:     make(chan PeerEvent, 100),
		stopChan:   make(chan struct{}),
		httpClient: &http.Client{Timeout: 1 * time.Second},
	}
}

func (ds *DiscoveryService) Events() <-chan PeerEvent {
	return ds.events
}

func (ds *DiscoveryService) Start() error {
	ds.localIPs = getLocalIPs()

	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", ds.cfg.DiscoveryPort))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP addr: %w", err)
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		// Fallback for secondary instances on the same host: bind dynamic UDP port
		dynAddr, _ := net.ResolveUDPAddr("udp4", ":0")
		conn, err = net.ListenUDP("udp4", dynAddr)
		if err != nil {
			return fmt.Errorf("failed to listen UDP: %w", err)
		}
	}
	ds.udpConn = conn

	// Start broadcast listener
	go ds.listenLoop()

	// Start periodic beacon broadcaster
	go ds.broadcastLoop()

	// Start local instance HTTP prober (for 100% reliable same-machine multi-instance testing)
	go ds.probeLoop()

	// Start peer cleanup housekeeper
	go ds.cleanupLoop()

	return nil
}

func (ds *DiscoveryService) Stop() {
	close(ds.stopChan)
	if ds.udpConn != nil {
		ds.udpConn.Close()
	}
}

func (ds *DiscoveryService) broadcastLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Initial immediate broadcast
	ds.sendBeacon()

	for {
		select {
		case <-ds.stopChan:
			return
		case <-ticker.C:
			ds.sendBeacon()
		}
	}
}

func (ds *DiscoveryService) sendBeacon() {
	payload := BeaconPayload{
		ID:        ds.cfg.DeviceID,
		Name:      ds.cfg.DeviceName,
		OS:        ds.cfg.OS,
		Port:      ds.cfg.UIPort,
		E2EE:      ds.cfg.E2EEEnabled,
		PublicKey: ds.cfg.SecretKey,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	// 1. Broadcast to LAN Subnet 255.255.255.255
	bcastAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", ds.cfg.DiscoveryPort))
	if err == nil {
		_, _ = ds.udpConn.WriteToUDP(data, bcastAddr)
	}

	// 2. Broadcast to Unicast localhost (127.0.0.1) on DiscoveryPort
	localUDP, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%d", ds.cfg.DiscoveryPort))
	if err == nil {
		_, _ = ds.udpConn.WriteToUDP(data, localUDP)
	}

	// 3. Send to interface-specific broadcast addresses
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipNet, ok := addr.(*net.IPNet)
				if !ok || ipNet.IP.To4() == nil {
					continue
				}

				ip := ipNet.IP.To4()
				mask := ipNet.Mask
				if len(mask) == 4 {
					bcastIP := net.IPv4(
						ip[0]|^mask[0],
						ip[1]|^mask[1],
						ip[2]|^mask[2],
						ip[3]|^mask[3],
					)
					dest := &net.UDPAddr{IP: bcastIP, Port: ds.cfg.DiscoveryPort}
					_, _ = ds.udpConn.WriteToUDP(data, dest)
				}
			}
		}
	}
}

func (ds *DiscoveryService) listenLoop() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ds.stopChan:
			return
		default:
			n, remoteAddr, err := ds.udpConn.ReadFromUDP(buf)
			if err != nil {
				return
			}

			senderIP := remoteAddr.IP.String()

			var payload BeaconPayload
			if err := json.Unmarshal(buf[:n], &payload); err != nil {
				continue
			}

			// Don't process self device ID
			if payload.ID == ds.cfg.DeviceID {
				continue
			}

			ds.handlePeerBeacon(payload, senderIP)
		}
	}
}

func (ds *DiscoveryService) probeLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// Initial probe
	ds.probeLocalInstances()

	for {
		select {
		case <-ds.stopChan:
			return
		case <-ticker.C:
			ds.probeLocalInstances()
		}
	}
}

// Probes common local ports for other LANShare instances on the same host
func (ds *DiscoveryService) probeLocalInstances() {
	myPayload := BeaconPayload{
		ID:        ds.cfg.DeviceID,
		Name:      ds.cfg.DeviceName,
		OS:        ds.cfg.OS,
		Port:      ds.cfg.UIPort,
		E2EE:      ds.cfg.E2EEEnabled,
		PublicKey: ds.cfg.SecretKey,
	}
	myBytes, _ := json.Marshal(myPayload)

	// Scan local UI & Transfer ports (52638 through 52650)
	for port := 52638; port <= 52650; port++ {
		if port == ds.cfg.UIPort || port == ds.cfg.TransferPort {
			continue
		}

		go func(p int) {
			url := fmt.Sprintf("http://127.0.0.1:%d/api/beacon", p)
			req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(myBytes))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := ds.httpClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var peerPayload BeaconPayload
				if err := json.NewDecoder(resp.Body).Decode(&peerPayload); err == nil {
					if peerPayload.ID != ds.cfg.DeviceID && peerPayload.ID != "" {
						ds.handlePeerBeacon(peerPayload, "127.0.0.1")
					}
				}
			}
		}(port)
	}
}

func (ds *DiscoveryService) HandlePeerBeaconExternal(payload BeaconPayload, ip string) {
	ds.handlePeerBeacon(payload, ip)
}

func (ds *DiscoveryService) handlePeerBeacon(payload BeaconPayload, ip string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	existing, exists := ds.peers[payload.ID]
	now := time.Now()

	p := Peer{
		ID:        payload.ID,
		Name:      payload.Name,
		OS:        payload.OS,
		IP:        ip,
		Port:      payload.Port,
		E2EE:      payload.E2EE,
		PublicKey: payload.PublicKey,
		LastSeen:  now,
		IsOnline:  true,
	}

	ds.peers[payload.ID] = &p

	eventType := "updated"
	if !exists || !existing.IsOnline {
		eventType = "joined"
	}

	select {
	case ds.events <- PeerEvent{Type: eventType, Peer: p}:
	default:
	}
}

func (ds *DiscoveryService) cleanupLoop() {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ds.stopChan:
			return
		case <-ticker.C:
			ds.pruneStalePeers()
		}
	}
}

func (ds *DiscoveryService) pruneStalePeers() {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	now := time.Now()
	for id, peer := range ds.peers {
		if peer.IsOnline && now.Sub(peer.LastSeen) > 8*time.Second {
			peer.IsOnline = false
			select {
			case ds.events <- PeerEvent{Type: "left", Peer: *peer}:
			default:
			}
			delete(ds.peers, id)
		}
	}
}

func (ds *DiscoveryService) GetActivePeers() []Peer {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	list := make([]Peer, 0, len(ds.peers))
	for _, p := range ds.peers {
		if p.IsOnline {
			list = append(list, *p)
		}
	}
	return list
}

func (ds *DiscoveryService) isLocalIP(ip string) bool {
	for _, local := range ds.localIPs {
		if local == ip {
			return true
		}
	}
	return false
}

func getLocalIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				ips = append(ips, ip.String())
			}
		}
	}
	return ips
}

func (ds *DiscoveryService) GetLocalIP() string {
	ips := getLocalIPs()
	for _, ip := range ips {
		if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "172.") {
			return ip
		}
	}
	if len(ips) > 0 {
		return ips[0]
	}
	return "127.0.0.1"
}
