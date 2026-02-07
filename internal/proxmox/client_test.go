package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestServer creates an httptest.TLSServer and a Client pointing to it.
func newTestServer(t *testing.T, handler http.Handler) (*httptest.Server, *Client) {
	t.Helper()
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)

	client := &Client{
		baseURL:     ts.URL,
		node:        "pve",
		tokenID:     "user@pam!token",
		tokenSecret: "secret-uuid-1234",
		httpClient:  ts.Client(),
	}
	return ts, client
}

func TestAuthHeader(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/cluster/nextid", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "100"})
	})

	_, client := newTestServer(t, mux)
	_, err := client.AllocateCTID(context.Background())
	if err != nil {
		t.Fatalf("AllocateCTID: %v", err)
	}

	want := "PVEAPIToken=user@pam!token=secret-uuid-1234"
	if gotAuth != want {
		t.Errorf("auth header = %q, want %q", gotAuth, want)
	}
}

func TestAllocateCTID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/cluster/nextid", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "104"})
	})

	_, client := newTestServer(t, mux)
	id, err := client.AllocateCTID(context.Background())
	if err != nil {
		t.Fatalf("AllocateCTID: %v", err)
	}
	if id != 104 {
		t.Errorf("id = %d, want 104", id)
	}
}

func TestCreateContainer(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		r.ParseForm()
		gotBody = r.PostForm.Encode()
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "UPID:pve:00001:00000:00000:create:100:user@pam:"})
	})
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped", "exitstatus": "OK"},
		})
	})

	_, client := newTestServer(t, mux)
	err := client.Create(context.Background(), ContainerCreateOptions{
		CTID:         100,
		OSTemplate:   "local:vztmpl/debian-12.tar.zst",
		Storage:      "local-lvm",
		RootFSSize:   8,
		Cores:        2,
		MemoryMB:     1024,
		Bridge:       "vmbr0",
		Hostname:     "test-ct",
		Unprivileged: true,
		Pool:         "appstore",
		Tags:         "appstore;managed",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api2/json/nodes/pve/lxc" {
		t.Errorf("path = %s, want /api2/json/nodes/pve/lxc", gotPath)
	}
	// Verify form params
	if !strings.Contains(gotBody, "vmid=100") {
		t.Errorf("body should contain vmid=100: %s", gotBody)
	}
	if !strings.Contains(gotBody, "unprivileged=1") {
		t.Errorf("body should contain unprivileged=1: %s", gotBody)
	}
	if !strings.Contains(gotBody, "pool=appstore") {
		t.Errorf("body should contain pool=appstore: %s", gotBody)
	}
}

func TestStartContainer(t *testing.T) {
	var gotMethod, gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100/status/start", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "UPID:pve:start:100:"})
	})
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped", "exitstatus": "OK"},
		})
	})

	_, client := newTestServer(t, mux)
	err := client.Start(context.Background(), 100)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/api2/json/nodes/pve/lxc/100/status/start" {
		t.Errorf("path = %s", gotPath)
	}
}

func TestStopContainer(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100/status/stop", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "UPID:pve:stop:100:"})
	})
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped", "exitstatus": "OK"},
		})
	})

	_, client := newTestServer(t, mux)
	err := client.Stop(context.Background(), 100)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if gotPath != "/api2/json/nodes/pve/lxc/100/status/stop" {
		t.Errorf("path = %s", gotPath)
	}
}

func TestShutdownContainer(t *testing.T) {
	var gotBody string
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100/status/shutdown", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotBody = r.PostForm.Encode()
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "UPID:pve:shutdown:100:"})
	})
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped", "exitstatus": "OK"},
		})
	})

	_, client := newTestServer(t, mux)
	err := client.Shutdown(context.Background(), 100, 30)
	if err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if !strings.Contains(gotBody, "timeout=30") {
		t.Errorf("body should contain timeout=30: %s", gotBody)
	}
}

func TestDestroyContainer(t *testing.T) {
	var gotMethod, gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "UPID:pve:destroy:100:"})
	})
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped", "exitstatus": "OK"},
		})
	})

	_, client := newTestServer(t, mux)
	err := client.Destroy(context.Background(), 100)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/api2/json/nodes/pve/lxc/100" {
		t.Errorf("path = %s", gotPath)
	}
}

func TestContainerStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100/status/current", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "running"},
		})
	})

	_, client := newTestServer(t, mux)
	status, err := client.Status(context.Background(), 100)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "running" {
		t.Errorf("status = %q, want %q", status, "running")
	}
}

func TestTaskPolling(t *testing.T) {
	var pollCount int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100/status/start", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "UPID:pve:start:100:"})
	})
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&pollCount, 1)
		if count < 3 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{"status": "running"},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{"status": "stopped", "exitstatus": "OK"},
			})
		}
	})

	_, client := newTestServer(t, mux)
	err := client.Start(context.Background(), 100)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if atomic.LoadInt32(&pollCount) < 3 {
		t.Errorf("expected at least 3 polls, got %d", pollCount)
	}
}

func TestTaskFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/lxc/100/status/start", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "UPID:pve:start:100:"})
	})
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped", "exitstatus": "ERROR: start failed"},
		})
	})

	_, client := newTestServer(t, mux)
	err := client.Start(context.Background(), 100)
	if err == nil {
		t.Fatal("expected error for failed task")
	}
	if !strings.Contains(err.Error(), "start failed") {
		t.Errorf("error = %v, should mention start failed", err)
	}
}

func TestUnauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/cluster/nextid", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"data":null}`)
	})

	_, client := newTestServer(t, mux)
	_, err := client.AllocateCTID(context.Background())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	pErr, ok := err.(*ProxmoxError)
	if !ok {
		// The error is wrapped, unwrap it
		t.Logf("error type = %T: %v", err, err)
	} else if pErr.StatusCode != 401 {
		t.Errorf("status = %d, want 401", pErr.StatusCode)
	}
}

func TestServerError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/cluster/nextid", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"data":null,"errors":{"vmid":"already exists"}}`)
	})

	_, client := newTestServer(t, mux)
	_, err := client.AllocateCTID(context.Background())
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestListTemplates(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/storage/local/content", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("content") != "vztmpl" {
			t.Errorf("content param = %q, want %q", r.URL.Query().Get("content"), "vztmpl")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"volid": "local:vztmpl/debian-12-standard_12.12-1_amd64.tar.zst", "size": 123456},
				{"volid": "local:vztmpl/ubuntu-24.04-standard_24.04-1_amd64.tar.zst", "size": 234567},
			},
		})
	})

	_, client := newTestServer(t, mux)
	templates, err := client.ListTemplates(context.Background(), "local")
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("len = %d, want 2", len(templates))
	}
	if templates[0].Volid != "local:vztmpl/debian-12-standard_12.12-1_amd64.tar.zst" {
		t.Errorf("volid = %q", templates[0].Volid)
	}
}

func TestResolveTemplate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/storage/local/content", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"volid": "local:vztmpl/debian-12-standard_12.12-1_amd64.tar.zst", "size": 123456},
			},
		})
	})

	_, client := newTestServer(t, mux)
	tmpl := client.ResolveTemplate(context.Background(), "debian-12", "local")
	if tmpl != "local:vztmpl/debian-12-standard_12.12-1_amd64.tar.zst" {
		t.Errorf("resolved = %q", tmpl)
	}
}

func TestResolveTemplateFallback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/storage/local/content", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
	})

	_, client := newTestServer(t, mux)
	tmpl := client.ResolveTemplate(context.Background(), "alpine-3.19", "local")
	if tmpl != "local:vztmpl/alpine-3.19-standard_amd64.tar.zst" {
		t.Errorf("fallback = %q", tmpl)
	}
}

func TestTaskTimeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "running"},
		})
	})

	_, client := newTestServer(t, mux)
	err := client.WaitForTask(context.Background(), "UPID:test", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, should mention timeout", err)
	}
}

func TestNewClientValidation(t *testing.T) {
	_, err := NewClient(ClientConfig{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}

	_, err = NewClient(ClientConfig{BaseURL: "https://localhost:8006"})
	if err == nil {
		t.Fatal("expected error for missing node")
	}

	c, err := NewClient(ClientConfig{
		BaseURL:       "https://localhost:8006",
		Node:          "pve",
		TLSSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c == nil {
		t.Fatal("client should not be nil")
	}
}
