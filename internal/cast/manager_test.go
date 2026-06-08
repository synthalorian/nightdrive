package cast

import (
	"testing"
	"time"
)

// mockStore implements DeviceStore for testing
type mockStore struct {
	devices []CastDeviceRecord
}

func (m *mockStore) InsertCastDevice(name, devType, host string, port int, protocol, capabilities string) (int64, error) {
	id := int64(len(m.devices) + 1)
	m.devices = append(m.devices, CastDeviceRecord{
		ID:           id,
		Name:         name,
		Type:         devType,
		Host:         host,
		Port:         port,
		Protocol:     protocol,
		Capabilities: capabilities,
		IsActive:     false,
		LastSeen:     time.Now(),
	})
	return id, nil
}

func (m *mockStore) UpdateCastDeviceActivity(id int64, active bool) error {
	for i := range m.devices {
		if m.devices[i].ID == id {
			m.devices[i].IsActive = active
			return nil
		}
	}
	return nil
}

func (m *mockStore) CastDevices() ([]CastDeviceRecord, error) {
	return m.devices, nil
}

func (m *mockStore) DeleteCastDevice(id int64) error {
	var filtered []CastDeviceRecord
	for _, d := range m.devices {
		if d.ID != id {
			filtered = append(filtered, d)
		}
	}
	m.devices = filtered
	return nil
}

func TestNewManager(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)
	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
}

func TestRegisterDevice(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	device, err := mgr.RegisterDevice("Living Room", TypeChromecast, "192.168.1.100", 8009)
	if err != nil {
		t.Fatalf("register device: %v", err)
	}
	if device == nil {
		t.Fatal("expected device, got nil")
	}
	if device.Name != "Living Room" {
		t.Errorf("expected name 'Living Room', got %s", device.Name)
	}
	if device.Type != TypeChromecast {
		t.Errorf("expected type chromecast, got %s", device.Type)
	}
	if device.Host != "192.168.1.100" {
		t.Errorf("expected host '192.168.1.100', got %s", device.Host)
	}
	if device.Port != 8009 {
		t.Errorf("expected port 8009, got %d", device.Port)
	}
}

func TestGetDevices(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	mgr.RegisterDevice("Device 1", TypeChromecast, "192.168.1.100", 8009)
	mgr.RegisterDevice("Device 2", TypeAirPlay, "192.168.1.101", 7000)

	devices, err := mgr.GetDevices()
	if err != nil {
		t.Fatalf("get devices: %v", err)
	}
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}
}

func TestGetDevice(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	device, _ := mgr.RegisterDevice("Test Device", TypeChromecast, "192.168.1.100", 8009)

	found, err := mgr.GetDevice(device.ID)
	if err != nil {
		t.Fatalf("get device: %v", err)
	}
	if found.ID != device.ID {
		t.Errorf("expected id %d, got %d", device.ID, found.ID)
	}

	// Not found
	_, err = mgr.GetDevice(999)
	if err == nil {
		t.Error("expected error for nonexistent device")
	}
}

func TestUnregisterDevice(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	device, _ := mgr.RegisterDevice("To Remove", TypeChromecast, "192.168.1.100", 8009)

	if err := mgr.UnregisterDevice(device.ID); err != nil {
		t.Fatalf("unregister device: %v", err)
	}

	devices, _ := mgr.GetDevices()
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
}

func TestDiscover(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	devices, err := mgr.Discover()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	// Discovery is simplified and returns empty in test environment
	_ = devices
}

func TestCastToDevice_Unsupported(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	device, _ := mgr.RegisterDevice("Test", TypeDLNA, "192.168.1.100", 8009)

	err := mgr.CastToDevice(device.ID, "http://stream", "Title", 0)
	if err == nil {
		t.Error("expected error for unsupported device type")
	}
}

func TestStopPlayback(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	device, _ := mgr.RegisterDevice("Test", TypeChromecast, "192.168.1.100", 8009)

	if err := mgr.StopPlayback(device.ID); err != nil {
		t.Fatalf("stop playback: %v", err)
	}
}

func TestPause(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	device, _ := mgr.RegisterDevice("Test", TypeChromecast, "192.168.1.100", 8009)

	if err := mgr.Pause(device.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
}

func TestResume(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	device, _ := mgr.RegisterDevice("Test", TypeChromecast, "192.168.1.100", 8009)

	if err := mgr.Resume(device.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
}

func TestSeekDevice(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	device, _ := mgr.RegisterDevice("Test", TypeChromecast, "192.168.1.100", 8009)

	if err := mgr.SeekDevice(device.ID, 30.5); err != nil {
		t.Fatalf("seek: %v", err)
	}
}

func TestSetVolume(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	device, _ := mgr.RegisterDevice("Test", TypeChromecast, "192.168.1.100", 8009)

	if err := mgr.SetVolume(device.ID, 0.75); err != nil {
		t.Fatalf("set volume: %v", err)
	}
}

func TestGetStatus(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)

	device, _ := mgr.RegisterDevice("Test", TypeChromecast, "192.168.1.100", 8009)

	status, err := mgr.GetStatus(device.ID)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status["device"] != "Test" {
		t.Errorf("expected device 'Test', got %v", status["device"])
	}
}

func TestChromecastController(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)
	ctrl := NewChromecastController(mgr)

	device, _ := mgr.RegisterDevice("Test", TypeChromecast, "192.168.1.100", 8009)

	if err := ctrl.LaunchApp(device.ID, "YouTube"); err != nil {
		t.Fatalf("launch app: %v", err)
	}

	status, err := ctrl.GetMediaStatus(device.ID)
	if err != nil {
		t.Fatalf("get media status: %v", err)
	}
	if status == nil {
		t.Error("expected status map")
	}
}

func TestAirPlayController(t *testing.T) {
	store := &mockStore{}
	mgr := NewManager(store)
	ctrl := NewAirPlayController(mgr)

	if ctrl == nil {
		t.Fatal("expected non-nil AirPlay controller")
	}
}
