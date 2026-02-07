package engine

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists jobs and logs to SQLite.
type Store struct {
	db *sql.DB
}

// NewStore opens (or creates) the SQLite database at the given path.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrent reads
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
			id          TEXT PRIMARY KEY,
			type        TEXT NOT NULL,
			state       TEXT NOT NULL,
			app_id      TEXT NOT NULL,
			app_name    TEXT NOT NULL DEFAULT '',
			ctid        INTEGER DEFAULT 0,
			node        TEXT NOT NULL DEFAULT '',
			pool        TEXT NOT NULL DEFAULT '',
			storage     TEXT NOT NULL DEFAULT '',
			bridge      TEXT NOT NULL DEFAULT '',
			cores       INTEGER DEFAULT 0,
			memory_mb   INTEGER DEFAULT 0,
			disk_gb     INTEGER DEFAULT 0,
			inputs_json TEXT DEFAULT '{}',
			outputs_json TEXT DEFAULT '{}',
			error       TEXT DEFAULT '',
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL,
			completed_at TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS job_logs (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id    TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			level     TEXT NOT NULL,
			message   TEXT NOT NULL,
			FOREIGN KEY (job_id) REFERENCES jobs(id)
		);

		CREATE INDEX IF NOT EXISTS idx_job_logs_job_id ON job_logs(job_id);

		CREATE TABLE IF NOT EXISTS installs (
			id         TEXT PRIMARY KEY,
			app_id     TEXT NOT NULL,
			app_name   TEXT NOT NULL DEFAULT '',
			ctid       INTEGER NOT NULL,
			node       TEXT NOT NULL,
			pool       TEXT NOT NULL,
			status     TEXT NOT NULL DEFAULT 'running',
			created_at TEXT NOT NULL
		);
	`)
	return err
}

// CreateJob inserts a new job.
func (s *Store) CreateJob(job *Job) error {
	inputsJSON, _ := json.Marshal(job.Inputs)
	outputsJSON, _ := json.Marshal(job.Outputs)

	_, err := s.db.Exec(`
		INSERT INTO jobs (id, type, state, app_id, app_name, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, inputs_json, outputs_json, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Type, job.State, job.AppID, job.AppName, job.CTID,
		job.Node, job.Pool, job.Storage, job.Bridge,
		job.Cores, job.MemoryMB, job.DiskGB,
		string(inputsJSON), string(outputsJSON), job.Error,
		job.CreatedAt.Format(time.RFC3339), job.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

// UpdateJob updates a job's mutable fields.
func (s *Store) UpdateJob(job *Job) error {
	inputsJSON, _ := json.Marshal(job.Inputs)
	outputsJSON, _ := json.Marshal(job.Outputs)

	completedAt := ""
	if job.CompletedAt != nil {
		completedAt = job.CompletedAt.Format(time.RFC3339)
	}

	_, err := s.db.Exec(`
		UPDATE jobs SET state=?, ctid=?, inputs_json=?, outputs_json=?, error=?, updated_at=?, completed_at=?
		WHERE id=?`,
		job.State, job.CTID,
		string(inputsJSON), string(outputsJSON), job.Error,
		job.UpdatedAt.Format(time.RFC3339), completedAt,
		job.ID,
	)
	return err
}

// GetJob retrieves a job by ID.
func (s *Store) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(`SELECT id, type, state, app_id, app_name, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, inputs_json, outputs_json, error, created_at, updated_at, completed_at FROM jobs WHERE id=?`, id)
	return scanJob(row)
}

// ListJobs returns all jobs, most recent first.
func (s *Store) ListJobs() ([]*Job, error) {
	rows, err := s.db.Query(`SELECT id, type, state, app_id, app_name, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, inputs_json, outputs_json, error, created_at, updated_at, completed_at FROM jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		job, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// AppendLog adds a log entry for a job.
func (s *Store) AppendLog(entry *LogEntry) error {
	_, err := s.db.Exec(`INSERT INTO job_logs (job_id, timestamp, level, message) VALUES (?, ?, ?, ?)`,
		entry.JobID, entry.Timestamp.Format(time.RFC3339Nano), entry.Level, entry.Message,
	)
	return err
}

// GetLogs returns all log entries for a job.
func (s *Store) GetLogs(jobID string) ([]*LogEntry, error) {
	rows, err := s.db.Query(`SELECT job_id, timestamp, level, message FROM job_logs WHERE job_id=? ORDER BY id ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*LogEntry
	for rows.Next() {
		var entry LogEntry
		var ts string
		if err := rows.Scan(&entry.JobID, &ts, &entry.Level, &entry.Message); err != nil {
			return nil, err
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		logs = append(logs, &entry)
	}
	return logs, rows.Err()
}

// GetLogsSince returns log entries after a given count (for polling).
func (s *Store) GetLogsSince(jobID string, afterID int) ([]*LogEntry, int, error) {
	rows, err := s.db.Query(`SELECT id, job_id, timestamp, level, message FROM job_logs WHERE job_id=? AND id > ? ORDER BY id ASC`, jobID, afterID)
	if err != nil {
		return nil, afterID, err
	}
	defer rows.Close()

	var logs []*LogEntry
	lastID := afterID
	for rows.Next() {
		var entry LogEntry
		var ts string
		var id int
		if err := rows.Scan(&id, &entry.JobID, &ts, &entry.Level, &entry.Message); err != nil {
			return nil, lastID, err
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		logs = append(logs, &entry)
		lastID = id
	}
	return logs, lastID, rows.Err()
}

// CreateInstall records a completed installation.
func (s *Store) CreateInstall(inst *Install) error {
	_, err := s.db.Exec(`INSERT INTO installs (id, app_id, app_name, ctid, node, pool, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		inst.ID, inst.AppID, inst.AppName, inst.CTID, inst.Node, inst.Pool, inst.Status,
		inst.CreatedAt.Format(time.RFC3339),
	)
	return err
}

// ListInstalls returns all installations.
func (s *Store) ListInstalls() ([]*Install, error) {
	rows, err := s.db.Query(`SELECT id, app_id, app_name, ctid, node, pool, status, created_at FROM installs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var installs []*Install
	for rows.Next() {
		var inst Install
		var createdAt string
		if err := rows.Scan(&inst.ID, &inst.AppID, &inst.AppName, &inst.CTID, &inst.Node, &inst.Pool, &inst.Status, &createdAt); err != nil {
			return nil, err
		}
		inst.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		installs = append(installs, &inst)
	}
	return installs, rows.Err()
}

// scanJob scans a single job from a QueryRow result.
func scanJob(row *sql.Row) (*Job, error) {
	var job Job
	var inputsJSON, outputsJSON, createdAt, updatedAt, completedAt string

	err := row.Scan(&job.ID, &job.Type, &job.State, &job.AppID, &job.AppName,
		&job.CTID, &job.Node, &job.Pool, &job.Storage, &job.Bridge,
		&job.Cores, &job.MemoryMB, &job.DiskGB,
		&inputsJSON, &outputsJSON, &job.Error,
		&createdAt, &updatedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(inputsJSON), &job.Inputs)
	json.Unmarshal([]byte(outputsJSON), &job.Outputs)
	job.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	job.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if completedAt != "" {
		t, _ := time.Parse(time.RFC3339, completedAt)
		job.CompletedAt = &t
	}

	return &job, nil
}

// scanJobRow scans a single job from a Rows iterator.
func scanJobRow(rows *sql.Rows) (*Job, error) {
	var job Job
	var inputsJSON, outputsJSON, createdAt, updatedAt, completedAt string

	err := rows.Scan(&job.ID, &job.Type, &job.State, &job.AppID, &job.AppName,
		&job.CTID, &job.Node, &job.Pool, &job.Storage, &job.Bridge,
		&job.Cores, &job.MemoryMB, &job.DiskGB,
		&inputsJSON, &outputsJSON, &job.Error,
		&createdAt, &updatedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	json.Unmarshal([]byte(inputsJSON), &job.Inputs)
	json.Unmarshal([]byte(outputsJSON), &job.Outputs)
	job.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	job.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if completedAt != "" {
		t, _ := time.Parse(time.RFC3339, completedAt)
		job.CompletedAt = &t
	}

	return &job, nil
}
