package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// --- StatusDetail ---

func TestStatusDetail(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100/status/current", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"status":  "running",
				"uptime":  3600,
				"cpu":     0.25,
				"cpus":    4,
				"mem":     536870912,
				"maxmem":  1073741824,
				"disk":    1073741824,
				"maxdisk": 8589934592,
				"netin":   1024,
				"netout":  2048,
			},
		})
	})

	_, client := newTestServer(t, mux)
	detail, err := client.StatusDetail(context.Background(), 100)
	if err != nil {
		t.Fatalf("StatusDetail: %v", err)
	}
	if detail.Status != "running" {
		t.Errorf("status = %q, want %q", detail.Status, "running")
	}
	if detail.Uptime != 3600 {
		t.Errorf("uptime = %d, want 3600", detail.Uptime)
	}
	if detail.CPUs != 4 {
		t.Errorf("cpus = %d, want 4", detail.CPUs)
	}
}

func TestStatusDetailAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/999/status/current", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"data": nil})
	})

	_, client := newTestServer(t, mux)
	_, err := client.StatusDetail(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for API error")
	}
}

// --- Status stopped ---

func TestStatusStopped(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/101/status/current", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped"},
		})
	})

	_, client := newTestServer(t, mux)
	status, err := client.Status(context.Background(), 101)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "stopped" {
		t.Errorf("status = %q, want %q", status, "stopped")
	}
}

// --- Start API error ---

func TestStartAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100/status/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"data": nil})
	})

	_, client := newTestServer(t, mux)
	err := client.Start(context.Background(), 100)
	if err == nil {
		t.Fatal("expected error for start API failure")
	}
}

// --- Stop API error ---

func TestStopAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100/status/stop", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"data": nil})
	})

	_, client := newTestServer(t, mux)
	err := client.Stop(context.Background(), 100)
	if err == nil {
		t.Fatal("expected error for stop API failure")
	}
}

// --- ListContainers ---

func TestListContainers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"vmid": 100, "name": "web", "status": "running", "tags": "appstore"},
				{"vmid": 101, "name": "db", "status": "stopped", "tags": ""},
			},
		})
	})

	_, client := newTestServer(t, mux)
	containers, err := client.ListContainers(context.Background())
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("len = %d, want 2", len(containers))
	}
	if containers[0].Name != "web" {
		t.Errorf("name = %q, want %q", containers[0].Name, "web")
	}
	if containers[1].Status != "stopped" {
		t.Errorf("status = %q, want %q", containers[1].Status, "stopped")
	}
}

// --- GetConfig ---

func TestGetConfig(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100/config", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"hostname": "test",
				"cores":    2,
				"memory":   1024,
				"net0":     "name=eth0,bridge=vmbr0,hwaddr=AA:BB:CC:DD:EE:FF",
			},
		})
	})

	_, client := newTestServer(t, mux)
	config, err := client.GetConfig(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config["hostname"] != "test" {
		t.Errorf("hostname = %v, want test", config["hostname"])
	}
}
