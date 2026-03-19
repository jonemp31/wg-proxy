package state

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
)

type Device struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	WGPublicKey    string `json:"wg_public_key"`
	WGPrivateKey   string `json:"wg_private_key"`
	WGPresharedKey string `json:"wg_preshared_key"`
	WGIP           string `json:"wg_ip"`
	ProxyPort      int    `json:"proxy_port"`
	ProxyUser      string `json:"proxy_user"`
	ProxyPass      string `json:"proxy_pass"`
	PhoneProxyPort int    `json:"phone_proxy_port"`
	ClientConfig   string `json:"client_config"`
}

// DeviceIP calculates the WireGuard IP for a device ID in a /22 subnet.
// Server is 10.0.0.1, device 1 = 10.0.0.2, device 254 = 10.0.0.255, device 255 = 10.0.1.0, etc.
func DeviceIP(subnetBase string, id int) string {
	offset := id + 1 // server is offset 1, device 1 is offset 2
	thirdOctet := offset / 256
	fourthOctet := offset % 256
	return fmt.Sprintf("%s.%d.%d", subnetBase, thirdOctet, fourthOctet)
}

type Store struct {
	mu      sync.RWMutex
	path    string
	devices []Device
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.devices = []Device{}
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.devices)
}

func (s *Store) save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.devices, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

func (s *Store) Add(d Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices = append(s.devices, d)
	return s.save()
}

func (s *Store) Remove(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range s.devices {
		if d.ID == id {
			s.devices = append(s.devices[:i], s.devices[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("device %d not found", id)
}

func (s *Store) Get(id int) (Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.devices {
		if d.ID == id {
			return d, true
		}
	}
	return Device{}, false
}

func (s *Store) GetAll() []Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Device, len(s.devices))
	copy(result, s.devices)
	return result
}

func (s *Store) GetByPublicKey(pubKey string) (Device, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.devices {
		if d.WGPublicKey == pubKey {
			return d, true
		}
	}
	return Device{}, false
}

func (s *Store) NextID(maxDevices int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	used := make(map[int]bool)
	for _, d := range s.devices {
		used[d.ID] = true
	}
	for id := 1; id <= maxDevices; id++ {
		if !used[id] {
			return id
		}
	}
	return -1
}

func GeneratePassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}
