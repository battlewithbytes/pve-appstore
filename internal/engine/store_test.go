package engine

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func makeJob(id, appID string) *Job {
	now := time.Now().Truncate(time.Second)
	return &Job{
		ID:        id,
		Type:      JobTypeInstall,
		State:     StateQueued,
		AppID:     appID,
		AppName:   "Test App",
		CTID:      0,
		Node:      "node1",
		Pool:      "pool1",
		Storage:   "local-lvm",
		Bridge:    "vmbr0",
		Cores:     2,
		MemoryMB:  1024,
		DiskGB:    8,
		Inputs:    map[string]string{"key1": "val1"},
		Outputs:   map[string]string{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestNewStore(t *testing.T) {
	s := newTestStore(t)
	if s == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestNewStoreBadPath(t *testing.T) {
	_, err := NewStore("/nonexistent/dir/test.db")
	if err == nil {
		t.Fatal("expected error for bad path")
	}
}

func TestCreateAndGetJob(t *testing.T) {
	s := newTestStore(t)
	job := makeJob("job-1", "nginx")

	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	got, err := s.GetJob("job-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}

	if got.ID != "job-1" {
		t.Errorf("ID = %q, want %q", got.ID, "job-1")
	}
	if got.Type != JobTypeInstall {
		t.Errorf("Type = %q, want %q", got.Type, JobTypeInstall)
	}
	if got.State != StateQueued {
		t.Errorf("State = %q, want %q", got.State, StateQueued)
	}
	if got.AppID != "nginx" {
		t.Errorf("AppID = %q, want %q", got.AppID, "nginx")
	}
	if got.AppName != "Test App" {
		t.Errorf("AppName = %q, want %q", got.AppName, "Test App")
	}
	if got.Node != "node1" {
		t.Errorf("Node = %q, want %q", got.Node, "node1")
	}
	if got.Pool != "pool1" {
		t.Errorf("Pool = %q, want %q", got.Pool, "pool1")
	}
	if got.Storage != "local-lvm" {
		t.Errorf("Storage = %q, want %q", got.Storage, "local-lvm")
	}
	if got.Bridge != "vmbr0" {
		t.Errorf("Bridge = %q, want %q", got.Bridge, "vmbr0")
	}
	if got.Cores != 2 {
		t.Errorf("Cores = %d, want %d", got.Cores, 2)
	}
	if got.MemoryMB != 1024 {
		t.Errorf("MemoryMB = %d, want %d", got.MemoryMB, 1024)
	}
	if got.DiskGB != 8 {
		t.Errorf("DiskGB = %d, want %d", got.DiskGB, 8)
	}
	if got.Inputs["key1"] != "val1" {
		t.Errorf("Inputs[key1] = %q, want %q", got.Inputs["key1"], "val1")
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil", got.CompletedAt)
	}
}

func TestGetJobNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetJob("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestCreateJobDuplicateID(t *testing.T) {
	s := newTestStore(t)
	job := makeJob("dup-1", "nginx")
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("first CreateJob: %v", err)
	}
	if err := s.CreateJob(job); err == nil {
		t.Fatal("expected error for duplicate job ID")
	}
}

func TestUpdateJob(t *testing.T) {
	s := newTestStore(t)
	job := makeJob("upd-1", "nginx")
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	// Update state, CTID, error
	job.State = StateFailed
	job.CTID = 100
	job.Error = "something broke"
	now := time.Now().Truncate(time.Second)
	job.UpdatedAt = now
	job.CompletedAt = &now
	job.Outputs = map[string]string{"url": "http://192.168.1.100"}

	if err := s.UpdateJob(job); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}

	got, err := s.GetJob("upd-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.State != StateFailed {
		t.Errorf("State = %q, want %q", got.State, StateFailed)
	}
	if got.CTID != 100 {
		t.Errorf("CTID = %d, want %d", got.CTID, 100)
	}
	if got.Error != "something broke" {
		t.Errorf("Error = %q, want %q", got.Error, "something broke")
	}
	if got.CompletedAt == nil {
		t.Fatal("CompletedAt should not be nil")
	}
	if got.Outputs["url"] != "http://192.168.1.100" {
		t.Errorf("Outputs[url] = %q, want %q", got.Outputs["url"], "http://192.168.1.100")
	}
}

func TestUpdateJobWithoutCompletedAt(t *testing.T) {
	s := newTestStore(t)
	job := makeJob("upd-2", "nginx")
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	job.State = StateCreateContainer
	job.CTID = 101
	job.UpdatedAt = time.Now()

	if err := s.UpdateJob(job); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}

	got, err := s.GetJob("upd-2")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.State != StateCreateContainer {
		t.Errorf("State = %q, want %q", got.State, StateCreateContainer)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt should be nil, got %v", got.CompletedAt)
	}
}

func TestListJobs(t *testing.T) {
	s := newTestStore(t)

	// Create jobs at different times
	j1 := makeJob("list-1", "nginx")
	j1.CreatedAt = time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	j1.UpdatedAt = j1.CreatedAt

	j2 := makeJob("list-2", "ollama")
	j2.CreatedAt = time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	j2.UpdatedAt = j2.CreatedAt

	j3 := makeJob("list-3", "plex")
	j3.CreatedAt = time.Now().Truncate(time.Second)
	j3.UpdatedAt = j3.CreatedAt

	for _, j := range []*Job{j1, j2, j3} {
		if err := s.CreateJob(j); err != nil {
			t.Fatalf("CreateJob(%s): %v", j.ID, err)
		}
	}

	jobs, err := s.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("len = %d, want 3", len(jobs))
	}
	// Most recent first
	if jobs[0].ID != "list-3" {
		t.Errorf("jobs[0].ID = %q, want %q", jobs[0].ID, "list-3")
	}
	if jobs[1].ID != "list-2" {
		t.Errorf("jobs[1].ID = %q, want %q", jobs[1].ID, "list-2")
	}
	if jobs[2].ID != "list-1" {
		t.Errorf("jobs[2].ID = %q, want %q", jobs[2].ID, "list-1")
	}
}

func TestListJobsEmpty(t *testing.T) {
	s := newTestStore(t)
	jobs, err := s.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if jobs != nil {
		t.Errorf("expected nil, got %d jobs", len(jobs))
	}
}

func TestAppendAndGetLogs(t *testing.T) {
	s := newTestStore(t)

	// Create a job first (for FK)
	job := makeJob("log-1", "nginx")
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	entries := []*LogEntry{
		{JobID: "log-1", Timestamp: time.Now().Add(-2 * time.Second), Level: "info", Message: "Starting"},
		{JobID: "log-1", Timestamp: time.Now().Add(-1 * time.Second), Level: "info", Message: "Running"},
		{JobID: "log-1", Timestamp: time.Now(), Level: "error", Message: "Failed"},
	}

	for _, e := range entries {
		if err := s.AppendLog(e); err != nil {
			t.Fatalf("AppendLog: %v", err)
		}
	}

	logs, err := s.GetLogs("log-1")
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("len = %d, want 3", len(logs))
	}
	if logs[0].Message != "Starting" {
		t.Errorf("logs[0].Message = %q, want %q", logs[0].Message, "Starting")
	}
	if logs[1].Message != "Running" {
		t.Errorf("logs[1].Message = %q, want %q", logs[1].Message, "Running")
	}
	if logs[2].Message != "Failed" {
		t.Errorf("logs[2].Message = %q, want %q", logs[2].Message, "Failed")
	}
	if logs[2].Level != "error" {
		t.Errorf("logs[2].Level = %q, want %q", logs[2].Level, "error")
	}
}

func TestGetLogsEmpty(t *testing.T) {
	s := newTestStore(t)
	job := makeJob("log-empty", "nginx")
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	logs, err := s.GetLogs("log-empty")
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if logs != nil {
		t.Errorf("expected nil, got %d logs", len(logs))
	}
}

func TestGetLogsSince(t *testing.T) {
	s := newTestStore(t)
	job := makeJob("since-1", "nginx")
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	// Append 5 log entries
	for i := 0; i < 5; i++ {
		if err := s.AppendLog(&LogEntry{
			JobID:     "since-1",
			Timestamp: time.Now(),
			Level:     "info",
			Message:   fmt.Sprintf("msg-%d", i),
		}); err != nil {
			t.Fatalf("AppendLog(%d): %v", i, err)
		}
	}

	// Get all (after=0)
	logs, lastID, err := s.GetLogsSince("since-1", 0)
	if err != nil {
		t.Fatalf("GetLogsSince(0): %v", err)
	}
	if len(logs) != 5 {
		t.Fatalf("len = %d, want 5", len(logs))
	}
	if lastID == 0 {
		t.Fatal("lastID should not be 0")
	}

	// Get after midpoint
	midID := lastID - 2
	logs2, lastID2, err := s.GetLogsSince("since-1", midID)
	if err != nil {
		t.Fatalf("GetLogsSince(%d): %v", midID, err)
	}
	if len(logs2) != 2 {
		t.Fatalf("len = %d, want 2", len(logs2))
	}
	if lastID2 != lastID {
		t.Errorf("lastID2 = %d, want %d", lastID2, lastID)
	}

	// Get after last — no new logs
	logs3, lastID3, err := s.GetLogsSince("since-1", lastID)
	if err != nil {
		t.Fatalf("GetLogsSince(%d): %v", lastID, err)
	}
	if logs3 != nil {
		t.Errorf("expected nil, got %d logs", len(logs3))
	}
	if lastID3 != lastID {
		t.Errorf("lastID3 = %d, want %d", lastID3, lastID)
	}
}

func TestCreateAndListInstalls(t *testing.T) {
	s := newTestStore(t)

	inst := &Install{
		ID:        "inst-1",
		AppID:     "nginx",
		AppName:   "Nginx",
		CTID:      100,
		Node:      "node1",
		Pool:      "pool1",
		Status:    "running",
		CreatedAt: time.Now().Truncate(time.Second),
	}
	if err := s.CreateInstall(inst); err != nil {
		t.Fatalf("CreateInstall: %v", err)
	}

	installs, err := s.ListInstalls()
	if err != nil {
		t.Fatalf("ListInstalls: %v", err)
	}
	if len(installs) != 1 {
		t.Fatalf("len = %d, want 1", len(installs))
	}

	got := installs[0]
	if got.ID != "inst-1" {
		t.Errorf("ID = %q, want %q", got.ID, "inst-1")
	}
	if got.AppID != "nginx" {
		t.Errorf("AppID = %q, want %q", got.AppID, "nginx")
	}
	if got.AppName != "Nginx" {
		t.Errorf("AppName = %q, want %q", got.AppName, "Nginx")
	}
	if got.CTID != 100 {
		t.Errorf("CTID = %d, want %d", got.CTID, 100)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want %q", got.Status, "running")
	}
}

func TestListInstallsEmpty(t *testing.T) {
	s := newTestStore(t)
	installs, err := s.ListInstalls()
	if err != nil {
		t.Fatalf("ListInstalls: %v", err)
	}
	if installs != nil {
		t.Errorf("expected nil, got %d installs", len(installs))
	}
}

func TestListInstallsOrder(t *testing.T) {
	s := newTestStore(t)

	i1 := &Install{ID: "i1", AppID: "a", AppName: "A", CTID: 100, Node: "n", Pool: "p", Status: "running",
		CreatedAt: time.Now().Add(-1 * time.Hour).Truncate(time.Second)}
	i2 := &Install{ID: "i2", AppID: "b", AppName: "B", CTID: 101, Node: "n", Pool: "p", Status: "running",
		CreatedAt: time.Now().Truncate(time.Second)}

	for _, inst := range []*Install{i1, i2} {
		if err := s.CreateInstall(inst); err != nil {
			t.Fatalf("CreateInstall(%s): %v", inst.ID, err)
		}
	}

	installs, err := s.ListInstalls()
	if err != nil {
		t.Fatalf("ListInstalls: %v", err)
	}
	if len(installs) != 2 {
		t.Fatalf("len = %d, want 2", len(installs))
	}
	// Most recent first
	if installs[0].ID != "i2" {
		t.Errorf("installs[0].ID = %q, want %q", installs[0].ID, "i2")
	}
}

func TestJobNilInputsOutputs(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	job := &Job{
		ID:        "nil-io",
		Type:      JobTypeInstall,
		State:     StateQueued,
		AppID:     "test",
		AppName:   "Test",
		Node:      "n",
		Pool:      "p",
		Storage:   "s",
		Bridge:    "b",
		CreatedAt: now,
		UpdatedAt: now,
		// Inputs and Outputs are nil
	}
	if err := s.CreateJob(job); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	got, err := s.GetJob("nil-io")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	// nil maps should round-trip as nil (from json "null")
	if got.Inputs != nil && len(got.Inputs) != 0 {
		t.Errorf("Inputs should be nil or empty, got %v", got.Inputs)
	}
}

func TestStoreMultipleLogJobs(t *testing.T) {
	s := newTestStore(t)

	j1 := makeJob("multi-1", "a")
	j2 := makeJob("multi-2", "b")
	s.CreateJob(j1)
	s.CreateJob(j2)

	s.AppendLog(&LogEntry{JobID: "multi-1", Timestamp: time.Now(), Level: "info", Message: "log for job 1"})
	s.AppendLog(&LogEntry{JobID: "multi-2", Timestamp: time.Now(), Level: "info", Message: "log for job 2"})
	s.AppendLog(&LogEntry{JobID: "multi-1", Timestamp: time.Now(), Level: "info", Message: "another log for job 1"})

	logs1, err := s.GetLogs("multi-1")
	if err != nil {
		t.Fatalf("GetLogs(multi-1): %v", err)
	}
	if len(logs1) != 2 {
		t.Errorf("job 1 logs: got %d, want 2", len(logs1))
	}

	logs2, err := s.GetLogs("multi-2")
	if err != nil {
		t.Fatalf("GetLogs(multi-2): %v", err)
	}
	if len(logs2) != 1 {
		t.Errorf("job 2 logs: got %d, want 1", len(logs2))
	}
}
