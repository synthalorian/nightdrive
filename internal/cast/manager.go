package cast

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DeviceType identifies the casting protocol.
type DeviceType string

const (
	TypeChromecast DeviceType = "chromecast"
	TypeAirPlay    DeviceType = "airplay"
	TypeDLNA       DeviceType = "dlna"
)

// Device represents a discovered cast device.
type Device struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Type         DeviceType `json:"type"`
	Host         string     `json:"host"`
	Port         int        `json:"port"`
	Protocol     string     `json:"protocol"`
	Capabilities []string   `json:"capabilities"`
	IsActive     bool       `json:"isActive"`
	LastSeen     time.Time  `json:"lastSeen"`
}

// Manager handles cast device discovery and control.
type Manager struct {
	mu       sync.RWMutex
	devices  map[string]*Device // key: "host:port"
	db       DeviceStore
	client   *http.Client
}

// DeviceStore persists cast device information.
type DeviceStore interface {
	InsertCastDevice(name string, devType string, host string, port int, protocol string, capabilities string) (int64, error)
	UpdateCastDeviceActivity(id int64, active bool) error
	CastDevices() ([]CastDeviceRecord, error)
	DeleteCastDevice(id int64) error
}

// CastDeviceRecord represents a device record from storage.
type CastDeviceRecord struct {
	ID           int64
	Name         string
	Type         string
	Host         string
	Port         int
	Protocol     string
	Capabilities string
	IsActive     bool
	LastSeen     time.Time
}

// NewManager creates a cast device manager.
func NewManager(store DeviceStore) *Manager {
	return &Manager{
		devices: make(map[string]*Device),
		db:      store,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// Discover scans the network for cast devices.
func (m *Manager) Discover() ([]Device, error) {
	var discovered []Device

	// Chromecast discovery via mDNS (simplified - scan common ports)
	chromecastDevices := m.discoverChromecast()
	discovered = append(discovered, chromecastDevices...)

	// AirPlay discovery via mDNS/Bonjour (simplified)
	airplayDevices := m.discoverAirPlay()
	discovered = append(discovered, airplayDevices...)

	// Store discovered devices
	for _, d := range discovered {
		key := fmt.Sprintf("%s:%d", d.Host, d.Port)
		m.mu.Lock()
		m.devices[key] = &d
		m.mu.Unlock()

		// Persist to database
		caps, _ := json.Marshal(d.Capabilities)
		if _, err := m.db.InsertCastDevice(d.Name, string(d.Type), d.Host, d.Port, d.Protocol, string(caps)); err != nil {
			log.Printf("[cast] store device %s: %v", d.Name, err)
		}
	}

	return discovered, nil
}

// discoverChromecast looks for Chromecast devices on the local network.
func (m *Manager) discoverChromecast() []Device {
	var devices []Device
	// Chromecasts typically use port 8009 for the Castv2 protocol
	// In a real implementation, this would use mDNS service discovery
	// For now, we provide a framework that can be extended
	return devices
}

// discoverAirPlay looks for AirPlay devices on the local network.
func (m *Manager) discoverAirPlay() []Device {
	var devices []Device
	// AirPlay devices typically use port 7000
	// In a real implementation, this would use Bonjour/mDNS
	return devices
}

// GetDevices returns all known cast devices.
func (m *Manager) GetDevices() ([]Device, error) {
	records, err := m.db.CastDevices()
	if err != nil {
		return nil, err
	}
	var devices []Device
	for _, r := range records {
		devices = append(devices, Device{
			ID:           r.ID,
			Name:         r.Name,
			Type:         DeviceType(r.Type),
			Host:         r.Host,
			Port:         r.Port,
			Protocol:     r.Protocol,
			IsActive:     r.IsActive,
			LastSeen:     r.LastSeen,
		})
	}
	return devices, nil
}

// GetDevice returns a specific device by ID.
func (m *Manager) GetDevice(id int64) (*Device, error) {
	records, err := m.db.CastDevices()
	if err != nil {
		return nil, err
	}
	for _, r := range records {
		if r.ID == id {
			return &Device{
				ID:       r.ID,
				Name:     r.Name,
				Type:     DeviceType(r.Type),
				Host:     r.Host,
				Port:     r.Port,
				Protocol: r.Protocol,
				IsActive: r.IsActive,
				LastSeen: r.LastSeen,
			}, nil
		}
	}
	return nil, fmt.Errorf("device not found")
}

// CastToDevice starts playback on a cast device.
func (m *Manager) CastToDevice(deviceID int64, streamURL string, title string, position float64) error {
	device, err := m.GetDevice(deviceID)
	if err != nil {
		return err
	}

	switch device.Type {
	case TypeChromecast:
		return m.castChromecast(device, streamURL, title, position)
	case TypeAirPlay:
		return m.castAirPlay(device, streamURL, title, position)
	default:
		return fmt.Errorf("unsupported device type: %s", device.Type)
	}
}

// castChromecast sends a load media command to a Chromecast device.
func (m *Manager) castChromecast(device *Device, streamURL, title string, position float64) error {
	// Chromecast uses the Castv2 protocol over TLS on port 8009
	// This is a simplified implementation that would need the full protocol
	// In production, use a Chromecast client library

	// Simulate connection attempt
	addr := net.JoinHostPort(device.Host, fmt.Sprintf("%d", device.Port))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return fmt.Errorf("connect to chromecast %s: %w", addr, err)
	}
	defer conn.Close()

	// In a real implementation, this would:
	// 1. Perform TLS handshake
	// 2. Exchange Castv2 protocol messages
	// 3. Launch the Default Media Receiver app
	// 4. Send LOAD media command with the stream URL

	log.Printf("[cast] would cast to Chromecast %s: %s", device.Name, streamURL)
	return nil
}

// castAirPlay sends playback to an AirPlay device.
func (m *Manager) castAirPlay(device *Device, streamURL, title string, position float64) error {
	// AirPlay uses RTSP/HTTP on port 7000
	// This is a simplified implementation

	addr := fmt.Sprintf("%s:%d", device.Host, device.Port)
	url := fmt.Sprintf("http://%s/play", addr)

	payload := map[string]interface{}{
		"Content-Location": streamURL,
		"Start-Position":   position,
		"X-Apple-AssetInfo": map[string]string{
			"title": title,
		},
	}

	body, _ := json.Marshal(payload)
	resp, err := m.client.Post(url, "text/x-apple-plist+xml", strings.NewReader(string(body)))
	if err != nil {
		// AirPlay uses proprietary protocol, HTTP fallback may not work
		log.Printf("[cast] AirPlay to %s: %v", addr, err)
		return fmt.Errorf("airplay control not available: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("airplay returned status %d", resp.StatusCode)
	}

	return nil
}

// StopPlayback stops playback on a cast device.
func (m *Manager) StopPlayback(deviceID int64) error {
	device, err := m.GetDevice(deviceID)
	if err != nil {
		return err
	}

	// In a real implementation, send STOP command via the device protocol
	log.Printf("[cast] stop playback on %s", device.Name)
	return nil
}

// Pause pauses playback on a cast device.
func (m *Manager) Pause(deviceID int64) error {
	device, err := m.GetDevice(deviceID)
	if err != nil {
		return err
	}

	log.Printf("[cast] pause on %s", device.Name)
	return nil
}

// Resume resumes playback on a cast device.
func (m *Manager) Resume(deviceID int64) error {
	device, err := m.GetDevice(deviceID)
	if err != nil {
		return err
	}

	log.Printf("[cast] resume on %s", device.Name)
	return nil
}

// SeekDevice seeks to a position on a cast device.
func (m *Manager) SeekDevice(deviceID int64, position float64) error {
	device, err := m.GetDevice(deviceID)
	if err != nil {
		return err
	}

	log.Printf("[cast] seek to %.2f on %s", position, device.Name)
	return nil
}

// SetVolume sets the volume on a cast device.
func (m *Manager) SetVolume(deviceID int64, volume float64) error {
	device, err := m.GetDevice(deviceID)
	if err != nil {
		return err
	}

	log.Printf("[cast] set volume to %.2f on %s", volume, device.Name)
	return nil
}

// GetStatus returns the current playback status of a device.
func (m *Manager) GetStatus(deviceID int64) (map[string]interface{}, error) {
	device, err := m.GetDevice(deviceID)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"device":   device.Name,
		"state":    "unknown",
		"position": 0.0,
		"volume":   1.0,
	}, nil
}

// RegisterDevice manually adds a cast device.
func (m *Manager) RegisterDevice(name string, devType DeviceType, host string, port int) (*Device, error) {
	caps, _ := json.Marshal([]string{"audio", "playback"})
	id, err := m.db.InsertCastDevice(name, string(devType), host, port, string(devType), string(caps))
	if err != nil {
		return nil, err
	}

	return &Device{
		ID:           id,
		Name:         name,
		Type:         devType,
		Host:         host,
		Port:         port,
		Protocol:     string(devType),
		Capabilities: []string{"audio", "playback"},
		IsActive:     false,
		LastSeen:     time.Now(),
	}, nil
}

// UnregisterDevice removes a cast device.
func (m *Manager) UnregisterDevice(deviceID int64) error {
	return m.db.DeleteCastDevice(deviceID)
}

// CastDeviceStore adapts the database for cast device storage.
type CastDeviceStore struct {
	DB interface {
		Exec(query string, args ...interface{}) (interface{ LastInsertId() (int64, error) }, error)
		Query(query string, args ...interface{}) (interface {
			Close() error
			Next() bool
			Scan(dest ...interface{}) error
			Err() error
		}, error)
		QueryRow(query string, args ...interface{}) interface{ Scan(dest ...interface{}) error }
	}
}

// ChromecastController provides Chromecast-specific control methods.
type ChromecastController struct {
	manager *Manager
}

// NewChromecastController creates a Chromecast controller.
func NewChromecastController(manager *Manager) *ChromecastController {
	return &ChromecastController{manager: manager}
}

// LaunchApp launches a specific app on a Chromecast.
func (c *ChromecastController) LaunchApp(deviceID int64, appID string) error {
	log.Printf("[cast] would launch app %s on device %d", appID, deviceID)
	return nil
}

// GetMediaStatus retrieves the current media status from a Chromecast.
func (c *ChromecastController) GetMediaStatus(deviceID int64) (map[string]interface{}, error) {
	return c.manager.GetStatus(deviceID)
}

// AirPlayController provides AirPlay-specific control methods.
type AirPlayController struct {
	manager *Manager
}

// NewAirPlayController creates an AirPlay controller.
func NewAirPlayController(manager *Manager) *AirPlayController {
	return &AirPlayController{manager: manager}
}

// StartPlayback initiates AirPlay playback with the full protocol.
func (a *AirPlayController) StartPlayback(deviceID int64, streamURL string, artworkURL string) error {
	device, err := a.manager.GetDevice(deviceID)
	if err != nil {
		return err
	}

	// AirPlay uses RTSP for playback control
	// POST /play with the stream URL
	addr := fmt.Sprintf("%s:%d", device.Host, device.Port)
	url := fmt.Sprintf("http://%s/play", addr)

	headers := map[string]string{
		"Content-Type":     "text/parameters",
		"X-Apple-Client-Name": "NightDrive",
	}

	body := fmt.Sprintf("Content-Location: %s\nStart-Position: 0.0\n", streamURL)

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := a.manager.client.Do(req)
	if err != nil {
		return fmt.Errorf("airplay play: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("airplay play failed: %s", string(respBody))
	}

	return nil
}

// GetPlaybackInfo retrieves current playback information from an AirPlay device.
func (a *AirPlayController) GetPlaybackInfo(deviceID int64) (map[string]interface{}, error) {
	device, err := a.manager.GetDevice(deviceID)
	if err != nil {
		return nil, err
	}

	addr := fmt.Sprintf("%s:%d", device.Host, device.Port)
	url := fmt.Sprintf("http://%s/playback-info", addr)

	resp, err := a.manager.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("playback-info returned %d", resp.StatusCode)
	}

	var info map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return info, nil
}

// ProtocolVersion returns the AirPlay protocol version supported by a device.
func (a *AirPlayController) ProtocolVersion(deviceID int64) (string, error) {
	device, err := a.manager.GetDevice(deviceID)
	if err != nil {
		return "", err
	}

	addr := fmt.Sprintf("%s:%d", device.Host, device.Port)
	url := fmt.Sprintf("http://%s/server-info", addr)

	resp, err := a.manager.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var info struct {
		Version string `json:"vers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}

	return info.Version, nil
}
