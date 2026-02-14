package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestWaitForTaskSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped", "exitstatus": "OK"},
		})
	})

	_, client := newTestServer(t, mux)
	err := client.WaitForTask(context.Background(), "UPID:test:ok", 5*time.Second)
	if err != nil {
		t.Fatalf("WaitForTask: %v", err)
	}
}

func TestWaitForTaskFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "stopped", "exitstatus": "ERROR: some failure"},
		})
	})

	_, client := newTestServer(t, mux)
	err := client.WaitForTask(context.Background(), "UPID:test:fail", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for failed task")
	}
	if !strings.Contains(err.Error(), "some failure") {
		t.Errorf("error = %v, should contain failure message", err)
	}
}

func TestWaitForTaskTimeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "running"},
		})
	})

	_, client := newTestServer(t, mux)
	err := client.WaitForTask(context.Background(), "UPID:test:slow", 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error = %v, should mention timed out", err)
	}
}

func TestWaitForTaskContextCancel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "running"},
		})
	})

	_, client := newTestServer(t, mux)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := client.WaitForTask(ctx, "UPID:test:cancel", 5*time.Second)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestWaitForTaskAPIError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api2/json/nodes/pve/tasks/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"data": nil})
	})

	_, client := newTestServer(t, mux)
	err := client.WaitForTask(context.Background(), "UPID:test:err", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}
