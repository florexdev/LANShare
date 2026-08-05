package discovery

import "time"

type Peer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OS        string    `json:"os"`
	IP        string    `json:"ip"`
	Port      int       `json:"port"`
	E2EE      bool      `json:"e2ee"`
	PublicKey string    `json:"public_key,omitempty"`
	LastSeen  time.Time `json:"last_seen"`
	IsOnline  bool      `json:"is_online"`
}

type PeerEvent struct {
	Type string `json:"type"` // "joined", "updated", "left"
	Peer Peer   `json:"peer"`
}
