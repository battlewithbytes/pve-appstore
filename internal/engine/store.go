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
	// Set pragmas via DSN so EVERY connection in the pool gets them.
	// database/sql pools connections — a PRAGMA run via db.Exec only
	// applies to one connection, leaving others without busy_timeout.
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite supports only one writer at a time. Limit the pool so
	// goroutines queue at the Go level instead of fighting over the lock.
	db.SetMaxOpenConns(4)

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
	if err != nil {
		return err
	}

	// Stacks table
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS stacks (
			id            TEXT PRIMARY KEY,
			name          TEXT NOT NULL,
			ctid          INTEGER NOT NULL DEFAULT 0,
			node          TEXT NOT NULL DEFAULT '',
			pool          TEXT NOT NULL DEFAULT '',
			storage       TEXT NOT NULL DEFAULT '',
			bridge        TEXT NOT NULL DEFAULT '',
			cores         INTEGER NOT NULL DEFAULT 0,
			memory_mb     INTEGER NOT NULL DEFAULT 0,
			disk_gb       INTEGER NOT NULL DEFAULT 0,
			hostname      TEXT NOT NULL DEFAULT '',
			onboot        INTEGER NOT NULL DEFAULT 1,
			unprivileged  INTEGER NOT NULL DEFAULT 1,
			ostemplate    TEXT NOT NULL DEFAULT '',
			apps_json     TEXT NOT NULL DEFAULT '[]',
			mounts_json   TEXT NOT NULL DEFAULT '[]',
			devices_json  TEXT NOT NULL DEFAULT '[]',
			env_vars_json TEXT NOT NULL DEFAULT '{}',
			status        TEXT NOT NULL DEFAULT 'running',
			created_at    TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	// GitHub state table for developer mode OAuth
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS github_state (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	// Idempotent migrations for enriched install fields
	alterStmts := []string{
		"ALTER TABLE installs ADD COLUMN app_version TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE installs ADD COLUMN storage TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE installs ADD COLUMN bridge TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE installs ADD COLUMN cores INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE installs ADD COLUMN memory_mb INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE installs ADD COLUMN disk_gb INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE installs ADD COLUMN outputs_json TEXT NOT NULL DEFAULT '{}'",
		"ALTER TABLE installs ADD COLUMN mounts_json TEXT NOT NULL DEFAULT '[]'",
		"ALTER TABLE jobs ADD COLUMN mounts_json TEXT NOT NULL DEFAULT '[]'",
		"ALTER TABLE jobs ADD COLUMN hostname TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE jobs ADD COLUMN onboot INTEGER NOT NULL DEFAULT 1",
		"ALTER TABLE jobs ADD COLUMN unprivileged INTEGER NOT NULL DEFAULT 1",
		// M3: persist install details for export/apply
		"ALTER TABLE installs ADD COLUMN hostname TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE installs ADD COLUMN onboot INTEGER NOT NULL DEFAULT 1",
		"ALTER TABLE installs ADD COLUMN unprivileged INTEGER NOT NULL DEFAULT 1",
		"ALTER TABLE installs ADD COLUMN inputs_json TEXT NOT NULL DEFAULT '{}'",
		"ALTER TABLE installs ADD COLUMN devices_json TEXT NOT NULL DEFAULT '[]'",
		"ALTER TABLE installs ADD COLUMN env_vars_json TEXT NOT NULL DEFAULT '{}'",
		"ALTER TABLE jobs ADD COLUMN devices_json TEXT NOT NULL DEFAULT '[]'",
		"ALTER TABLE jobs ADD COLUMN env_vars_json TEXT NOT NULL DEFAULT '{}'",
		"ALTER TABLE jobs ADD COLUMN stack_id TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE jobs ADD COLUMN ip_address TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE installs ADD COLUMN ip_address TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE stacks ADD COLUMN ip_address TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE jobs ADD COLUMN mac_address TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE installs ADD COLUMN mac_address TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE stacks ADD COLUMN mac_address TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE jobs ADD COLUMN app_source TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE installs ADD COLUMN app_source TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE jobs ADD COLUMN cpu_pin TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE installs ADD COLUMN cpu_pin TEXT NOT NULL DEFAULT ''",
	}
	for _, stmt := range alterStmts {
		s.db.Exec(stmt) // ignore "duplicate column" errors
	}

	return nil
}

// CreateJob inserts a new job.
func (s *Store) CreateJob(job *Job) error {
	inputsJSON, _ := json.Marshal(job.Inputs)
	outputsJSON, _ := json.Marshal(job.Outputs)
	mountsJSON, _ := json.Marshal(job.MountPoints)
	if job.MountPoints == nil {
		mountsJSON = []byte("[]")
	}
	devicesJSON, _ := json.Marshal(job.Devices)
	if job.Devices == nil {
		devicesJSON = []byte("[]")
	}
	envVarsJSON, _ := json.Marshal(job.EnvVars)
	if job.EnvVars == nil {
		envVarsJSON = []byte("{}")
	}

	_, err := s.db.Exec(`
		INSERT INTO jobs (id, type, state, app_id, app_name, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, inputs_json, outputs_json, mounts_json, devices_json, env_vars_json, stack_id, app_source, cpu_pin, error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Type, job.State, job.AppID, job.AppName, job.CTID,
		job.Node, job.Pool, job.Storage, job.Bridge,
		job.Cores, job.MemoryMB, job.DiskGB,
		job.Hostname, job.IPAddress, job.MACAddress, boolToInt(job.OnBoot), boolToInt(job.Unprivileged),
		string(inputsJSON), string(outputsJSON), string(mountsJSON),
		string(devicesJSON), string(envVarsJSON), job.StackID, job.AppSource, job.CPUPin, job.Error,
		job.CreatedAt.Format(time.RFC3339), job.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// UpdateJob updates a job's mutable fields.
func (s *Store) UpdateJob(job *Job) error {
	inputsJSON, _ := json.Marshal(job.Inputs)
	outputsJSON, _ := json.Marshal(job.Outputs)
	mountsJSON, _ := json.Marshal(job.MountPoints)
	if job.MountPoints == nil {
		mountsJSON = []byte("[]")
	}
	devicesJSON, _ := json.Marshal(job.Devices)
	if job.Devices == nil {
		devicesJSON = []byte("[]")
	}
	envVarsJSON, _ := json.Marshal(job.EnvVars)
	if job.EnvVars == nil {
		envVarsJSON = []byte("{}")
	}

	completedAt := ""
	if job.CompletedAt != nil {
		completedAt = job.CompletedAt.Format(time.RFC3339)
	}

	_, err := s.db.Exec(`
		UPDATE jobs SET state=?, ctid=?, hostname=?, ip_address=?, mac_address=?, onboot=?, unprivileged=?, inputs_json=?, outputs_json=?, mounts_json=?, devices_json=?, env_vars_json=?, cpu_pin=?, error=?, updated_at=?, completed_at=?
		WHERE id=?`,
		job.State, job.CTID,
		job.Hostname, job.IPAddress, job.MACAddress, boolToInt(job.OnBoot), boolToInt(job.Unprivileged),
		string(inputsJSON), string(outputsJSON), string(mountsJSON),
		string(devicesJSON), string(envVarsJSON), job.CPUPin, job.Error,
		job.UpdatedAt.Format(time.RFC3339), completedAt,
		job.ID,
	)
	return err
}

// GetJob retrieves a job by ID.
func (s *Store) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(`SELECT id, type, state, app_id, app_name, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, inputs_json, outputs_json, mounts_json, devices_json, env_vars_json, stack_id, app_source, cpu_pin, error, created_at, updated_at, completed_at FROM jobs WHERE id=?`, id)
	return scanJob(row)
}

// ListJobs returns all jobs, most recent first.
func (s *Store) ListJobs() ([]*Job, error) {
	rows, err := s.db.Query(`SELECT id, type, state, app_id, app_name, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, inputs_json, outputs_json, mounts_json, devices_json, env_vars_json, stack_id, app_source, cpu_pin, error, created_at, updated_at, completed_at FROM jobs ORDER BY created_at DESC`)
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
	outputsJSON, _ := json.Marshal(inst.Outputs)
	mountsJSON, _ := json.Marshal(inst.MountPoints)
	if inst.MountPoints == nil {
		mountsJSON = []byte("[]")
	}
	inputsJSON, _ := json.Marshal(inst.Inputs)
	if inst.Inputs == nil {
		inputsJSON = []byte("{}")
	}
	devicesJSON, _ := json.Marshal(inst.Devices)
	if inst.Devices == nil {
		devicesJSON = []byte("[]")
	}
	envVarsJSON, _ := json.Marshal(inst.EnvVars)
	if inst.EnvVars == nil {
		envVarsJSON = []byte("{}")
	}
	_, err := s.db.Exec(`INSERT INTO installs (id, app_id, app_name, app_version, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, inputs_json, outputs_json, mounts_json, devices_json, env_vars_json, app_source, cpu_pin, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inst.ID, inst.AppID, inst.AppName, inst.AppVersion, inst.CTID, inst.Node, inst.Pool,
		inst.Storage, inst.Bridge, inst.Cores, inst.MemoryMB, inst.DiskGB,
		inst.Hostname, inst.IPAddress, inst.MACAddress, boolToInt(inst.OnBoot), boolToInt(inst.Unprivileged),
		string(inputsJSON), string(outputsJSON), string(mountsJSON),
		string(devicesJSON), string(envVarsJSON), inst.AppSource, inst.CPUPin, inst.Status,
		inst.CreatedAt.Format(time.RFC3339),
	)
	return err
}

// UpdateInstall updates an install record (used for volume preservation and reinstall).
func (s *Store) UpdateInstall(inst *Install) error {
	outputsJSON, _ := json.Marshal(inst.Outputs)
	mountsJSON, _ := json.Marshal(inst.MountPoints)
	if inst.MountPoints == nil {
		mountsJSON = []byte("[]")
	}
	inputsJSON, _ := json.Marshal(inst.Inputs)
	if inst.Inputs == nil {
		inputsJSON = []byte("{}")
	}
	devicesJSON, _ := json.Marshal(inst.Devices)
	if inst.Devices == nil {
		devicesJSON = []byte("[]")
	}
	envVarsJSON, _ := json.Marshal(inst.EnvVars)
	if inst.EnvVars == nil {
		envVarsJSON = []byte("{}")
	}
	_, err := s.db.Exec(`UPDATE installs SET ctid=?, status=?, mounts_json=?, outputs_json=?, storage=?, bridge=?, cores=?, memory_mb=?, disk_gb=?, hostname=?, ip_address=?, mac_address=?, onboot=?, unprivileged=?, inputs_json=?, devices_json=?, env_vars_json=?, cpu_pin=? WHERE id=?`,
		inst.CTID, inst.Status, string(mountsJSON), string(outputsJSON),
		inst.Storage, inst.Bridge, inst.Cores, inst.MemoryMB, inst.DiskGB,
		inst.Hostname, inst.IPAddress, inst.MACAddress, boolToInt(inst.OnBoot), boolToInt(inst.Unprivileged),
		string(inputsJSON), string(devicesJSON), string(envVarsJSON), inst.CPUPin,
		inst.ID,
	)
	return err
}

// UpdateInstallStatus updates only the status of an install record.
func (s *Store) UpdateInstallStatus(id, status string) error {
	_, err := s.db.Exec(`UPDATE installs SET status=? WHERE id=?`, status, id)
	return err
}

// GetInstall retrieves a single install by ID.
func (s *Store) GetInstall(id string) (*Install, error) {
	row := s.db.QueryRow(`SELECT id, app_id, app_name, app_version, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, inputs_json, outputs_json, mounts_json, devices_json, env_vars_json, app_source, cpu_pin, status, created_at FROM installs WHERE id=?`, id)
	return scanInstallRow(row)
}

// ListInstalls returns all installations.
func (s *Store) ListInstalls() ([]*Install, error) {
	rows, err := s.db.Query(`SELECT id, app_id, app_name, app_version, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, inputs_json, outputs_json, mounts_json, devices_json, env_vars_json, app_source, cpu_pin, status, created_at FROM installs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var installs []*Install
	for rows.Next() {
		inst, err := scanInstallRows(rows)
		if err != nil {
			return nil, err
		}
		installs = append(installs, inst)
	}
	return installs, rows.Err()
}

func scanInstallRow(row *sql.Row) (*Install, error) {
	var inst Install
	var createdAt, inputsJSON, outputsJSON, mountsJSON, devicesJSON, envVarsJSON string
	var onboot, unprivileged int
	err := row.Scan(&inst.ID, &inst.AppID, &inst.AppName, &inst.AppVersion,
		&inst.CTID, &inst.Node, &inst.Pool,
		&inst.Storage, &inst.Bridge, &inst.Cores, &inst.MemoryMB, &inst.DiskGB,
		&inst.Hostname, &inst.IPAddress, &inst.MACAddress, &onboot, &unprivileged,
		&inputsJSON, &outputsJSON, &mountsJSON, &devicesJSON, &envVarsJSON,
		&inst.AppSource, &inst.CPUPin, &inst.Status, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	inst.OnBoot = onboot != 0
	inst.Unprivileged = unprivileged != 0
	json.Unmarshal([]byte(inputsJSON), &inst.Inputs)
	json.Unmarshal([]byte(outputsJSON), &inst.Outputs)
	json.Unmarshal([]byte(mountsJSON), &inst.MountPoints)
	json.Unmarshal([]byte(devicesJSON), &inst.Devices)
	json.Unmarshal([]byte(envVarsJSON), &inst.EnvVars)
	inst.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &inst, nil
}

func scanInstallRows(rows *sql.Rows) (*Install, error) {
	var inst Install
	var createdAt, inputsJSON, outputsJSON, mountsJSON, devicesJSON, envVarsJSON string
	var onboot, unprivileged int
	err := rows.Scan(&inst.ID, &inst.AppID, &inst.AppName, &inst.AppVersion,
		&inst.CTID, &inst.Node, &inst.Pool,
		&inst.Storage, &inst.Bridge, &inst.Cores, &inst.MemoryMB, &inst.DiskGB,
		&inst.Hostname, &inst.IPAddress, &inst.MACAddress, &onboot, &unprivileged,
		&inputsJSON, &outputsJSON, &mountsJSON, &devicesJSON, &envVarsJSON,
		&inst.AppSource, &inst.CPUPin, &inst.Status, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	inst.OnBoot = onboot != 0
	inst.Unprivileged = unprivileged != 0
	json.Unmarshal([]byte(inputsJSON), &inst.Inputs)
	json.Unmarshal([]byte(outputsJSON), &inst.Outputs)
	json.Unmarshal([]byte(mountsJSON), &inst.MountPoints)
	json.Unmarshal([]byte(devicesJSON), &inst.Devices)
	json.Unmarshal([]byte(envVarsJSON), &inst.EnvVars)
	inst.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &inst, nil
}

// scanJob scans a single job from a QueryRow result.
func scanJob(row *sql.Row) (*Job, error) {
	var job Job
	var inputsJSON, outputsJSON, mountsJSON, devicesJSON, envVarsJSON, createdAt, updatedAt, completedAt string
	var onboot, unprivileged int

	err := row.Scan(&job.ID, &job.Type, &job.State, &job.AppID, &job.AppName,
		&job.CTID, &job.Node, &job.Pool, &job.Storage, &job.Bridge,
		&job.Cores, &job.MemoryMB, &job.DiskGB,
		&job.Hostname, &job.IPAddress, &job.MACAddress, &onboot, &unprivileged,
		&inputsJSON, &outputsJSON, &mountsJSON, &devicesJSON, &envVarsJSON,
		&job.StackID, &job.AppSource, &job.CPUPin, &job.Error, &createdAt, &updatedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	job.OnBoot = onboot != 0
	job.Unprivileged = unprivileged != 0
	json.Unmarshal([]byte(inputsJSON), &job.Inputs)
	json.Unmarshal([]byte(outputsJSON), &job.Outputs)
	json.Unmarshal([]byte(mountsJSON), &job.MountPoints)
	json.Unmarshal([]byte(devicesJSON), &job.Devices)
	json.Unmarshal([]byte(envVarsJSON), &job.EnvVars)
	job.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	job.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if completedAt != "" {
		t, _ := time.Parse(time.RFC3339, completedAt)
		job.CompletedAt = &t
	}

	return &job, nil
}

// HasActiveJobForApp returns a non-terminal install job for the given app, if any.
func (s *Store) HasActiveJobForApp(appID string) (*Job, bool) {
	row := s.db.QueryRow(`SELECT id, type, state, app_id, app_name, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, inputs_json, outputs_json, mounts_json, devices_json, env_vars_json, stack_id, app_source, cpu_pin, error, created_at, updated_at, completed_at FROM jobs WHERE app_id=? AND type='install' AND state NOT IN ('completed','failed','cancelled') ORDER BY created_at DESC LIMIT 1`, appID)
	job, err := scanJob(row)
	if err != nil {
		return nil, false
	}
	return job, true
}

// HasActiveDevInstallForApp returns a non-uninstalled install for the given app
// that was installed from a developer source, if any.
func (s *Store) HasActiveDevInstallForApp(appID string) (*Install, bool) {
	row := s.db.QueryRow(`SELECT id, app_id, app_name, app_version, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, inputs_json, outputs_json, mounts_json, devices_json, env_vars_json, app_source, cpu_pin, status, created_at FROM installs WHERE app_id=? AND app_source='developer' AND status!='uninstalled' ORDER BY created_at DESC LIMIT 1`, appID)
	inst, err := scanInstallRow(row)
	if err != nil {
		return nil, false
	}
	return inst, true
}

// HasActiveInstallForApp returns a non-uninstalled install for the given app, if any.
func (s *Store) HasActiveInstallForApp(appID string) (*Install, bool) {
	row := s.db.QueryRow(`SELECT id, app_id, app_name, app_version, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, inputs_json, outputs_json, mounts_json, devices_json, env_vars_json, app_source, cpu_pin, status, created_at FROM installs WHERE app_id=? AND status!='uninstalled' ORDER BY created_at DESC LIMIT 1`, appID)
	inst, err := scanInstallRow(row)
	if err != nil {
		return nil, false
	}
	return inst, true
}

// OrphanedJob holds info about a job that was running when the service restarted.
type OrphanedJob struct {
	ID   string
	CTID int
}

// RecoverOrphanedJobs marks all non-terminal jobs as failed on startup.
// Returns the orphaned jobs (with CTIDs) so the engine can clean up containers.
func (s *Store) RecoverOrphanedJobs() ([]OrphanedJob, error) {
	now := time.Now().Format(time.RFC3339)

	// Find orphaned job IDs and CTIDs first
	rows, err := s.db.Query(`SELECT id, ctid FROM jobs WHERE state NOT IN ('completed','failed','cancelled')`)
	if err != nil {
		return nil, err
	}
	var orphans []OrphanedJob
	for rows.Next() {
		var o OrphanedJob
		if err := rows.Scan(&o.ID, &o.CTID); err == nil {
			orphans = append(orphans, o)
		}
	}
	rows.Close()

	if len(orphans) == 0 {
		return nil, nil
	}

	// Mark all as failed
	_, err = s.db.Exec(`UPDATE jobs SET state='failed', error='interrupted by service restart', completed_at=?, updated_at=? WHERE state NOT IN ('completed','failed','cancelled')`, now, now)
	if err != nil {
		return nil, err
	}

	// Append a log entry for each recovered job
	for _, o := range orphans {
		s.AppendLog(&LogEntry{
			JobID:     o.ID,
			Timestamp: time.Now(),
			Level:     "warn",
			Message:   "Job interrupted — service was restarted while this job was running",
		})
	}

	return orphans, nil
}

// ClearTerminalJobs deletes all jobs in a terminal state (completed, failed, cancelled)
// and their associated log entries. Returns the number of jobs deleted.
func (s *Store) ClearTerminalJobs() (int64, error) {
	// Delete logs for terminal jobs first (foreign key)
	_, err := s.db.Exec(`DELETE FROM job_logs WHERE job_id IN (
		SELECT id FROM jobs WHERE state IN ('completed','failed','cancelled')
	)`)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM jobs WHERE state IN ('completed','failed','cancelled')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// scanJobRow scans a single job from a Rows iterator.
func scanJobRow(rows *sql.Rows) (*Job, error) {
	var job Job
	var inputsJSON, outputsJSON, mountsJSON, devicesJSON, envVarsJSON, createdAt, updatedAt, completedAt string
	var onboot, unprivileged int

	err := rows.Scan(&job.ID, &job.Type, &job.State, &job.AppID, &job.AppName,
		&job.CTID, &job.Node, &job.Pool, &job.Storage, &job.Bridge,
		&job.Cores, &job.MemoryMB, &job.DiskGB,
		&job.Hostname, &job.IPAddress, &job.MACAddress, &onboot, &unprivileged,
		&inputsJSON, &outputsJSON, &mountsJSON, &devicesJSON, &envVarsJSON,
		&job.StackID, &job.AppSource, &job.CPUPin, &job.Error, &createdAt, &updatedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}

	job.OnBoot = onboot != 0
	job.Unprivileged = unprivileged != 0
	json.Unmarshal([]byte(inputsJSON), &job.Inputs)
	json.Unmarshal([]byte(outputsJSON), &job.Outputs)
	json.Unmarshal([]byte(mountsJSON), &job.MountPoints)
	json.Unmarshal([]byte(devicesJSON), &job.Devices)
	json.Unmarshal([]byte(envVarsJSON), &job.EnvVars)
	job.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	job.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if completedAt != "" {
		t, _ := time.Parse(time.RFC3339, completedAt)
		job.CompletedAt = &t
	}

	return &job, nil
}

// --- Stack CRUD ---

// CreateStack inserts a new stack record.
func (s *Store) CreateStack(stack *Stack) error {
	appsJSON, _ := json.Marshal(stack.Apps)
	mountsJSON, _ := json.Marshal(stack.MountPoints)
	if stack.MountPoints == nil {
		mountsJSON = []byte("[]")
	}
	devicesJSON, _ := json.Marshal(stack.Devices)
	if stack.Devices == nil {
		devicesJSON = []byte("[]")
	}
	envVarsJSON, _ := json.Marshal(stack.EnvVars)
	if stack.EnvVars == nil {
		envVarsJSON = []byte("{}")
	}

	_, err := s.db.Exec(`INSERT INTO stacks (id, name, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, ostemplate, apps_json, mounts_json, devices_json, env_vars_json, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		stack.ID, stack.Name, stack.CTID, stack.Node, stack.Pool,
		stack.Storage, stack.Bridge, stack.Cores, stack.MemoryMB, stack.DiskGB,
		stack.Hostname, stack.IPAddress, stack.MACAddress, boolToInt(stack.OnBoot), boolToInt(stack.Unprivileged),
		stack.OSTemplate, string(appsJSON), string(mountsJSON),
		string(devicesJSON), string(envVarsJSON), stack.Status,
		stack.CreatedAt.Format(time.RFC3339),
	)
	return err
}

// UpdateStack updates a stack record.
func (s *Store) UpdateStack(stack *Stack) error {
	appsJSON, _ := json.Marshal(stack.Apps)
	mountsJSON, _ := json.Marshal(stack.MountPoints)
	if stack.MountPoints == nil {
		mountsJSON = []byte("[]")
	}
	devicesJSON, _ := json.Marshal(stack.Devices)
	if stack.Devices == nil {
		devicesJSON = []byte("[]")
	}
	envVarsJSON, _ := json.Marshal(stack.EnvVars)
	if stack.EnvVars == nil {
		envVarsJSON = []byte("{}")
	}

	_, err := s.db.Exec(`UPDATE stacks SET ctid=?, status=?, apps_json=?, mounts_json=?, devices_json=?, env_vars_json=? WHERE id=?`,
		stack.CTID, stack.Status, string(appsJSON), string(mountsJSON),
		string(devicesJSON), string(envVarsJSON), stack.ID,
	)
	return err
}

// GetStack retrieves a single stack by ID.
func (s *Store) GetStack(id string) (*Stack, error) {
	row := s.db.QueryRow(`SELECT id, name, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, ostemplate, apps_json, mounts_json, devices_json, env_vars_json, status, created_at FROM stacks WHERE id=?`, id)
	return scanStackRow(row)
}

// ListStacks returns all stacks, most recent first.
func (s *Store) ListStacks() ([]*Stack, error) {
	rows, err := s.db.Query(`SELECT id, name, ctid, node, pool, storage, bridge, cores, memory_mb, disk_gb, hostname, ip_address, mac_address, onboot, unprivileged, ostemplate, apps_json, mounts_json, devices_json, env_vars_json, status, created_at FROM stacks ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stacks []*Stack
	for rows.Next() {
		stack, err := scanStackRows(rows)
		if err != nil {
			return nil, err
		}
		stacks = append(stacks, stack)
	}
	return stacks, rows.Err()
}

// DeleteStack removes a stack record.
func (s *Store) DeleteStack(id string) error {
	_, err := s.db.Exec("DELETE FROM stacks WHERE id=?", id)
	return err
}

func scanStackRow(row *sql.Row) (*Stack, error) {
	var stack Stack
	var appsJSON, mountsJSON, devicesJSON, envVarsJSON, createdAt string
	var onboot, unprivileged int
	err := row.Scan(&stack.ID, &stack.Name, &stack.CTID, &stack.Node, &stack.Pool,
		&stack.Storage, &stack.Bridge, &stack.Cores, &stack.MemoryMB, &stack.DiskGB,
		&stack.Hostname, &stack.IPAddress, &stack.MACAddress, &onboot, &unprivileged, &stack.OSTemplate,
		&appsJSON, &mountsJSON, &devicesJSON, &envVarsJSON,
		&stack.Status, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	stack.OnBoot = onboot != 0
	stack.Unprivileged = unprivileged != 0
	json.Unmarshal([]byte(appsJSON), &stack.Apps)
	json.Unmarshal([]byte(mountsJSON), &stack.MountPoints)
	json.Unmarshal([]byte(devicesJSON), &stack.Devices)
	json.Unmarshal([]byte(envVarsJSON), &stack.EnvVars)
	stack.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &stack, nil
}

// --- GitHub State ---

// SetGitHubState stores a key-value pair in the github_state table.
func (s *Store) SetGitHubState(key, value string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO github_state (key, value) VALUES (?, ?)`, key, value)
	return err
}

// GetGitHubState retrieves a value from the github_state table.
func (s *Store) GetGitHubState(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM github_state WHERE key=?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// DeleteGitHubState removes a key from the github_state table.
func (s *Store) DeleteGitHubState(key string) error {
	_, err := s.db.Exec(`DELETE FROM github_state WHERE key=?`, key)
	return err
}

func scanStackRows(rows *sql.Rows) (*Stack, error) {
	var stack Stack
	var appsJSON, mountsJSON, devicesJSON, envVarsJSON, createdAt string
	var onboot, unprivileged int
	err := rows.Scan(&stack.ID, &stack.Name, &stack.CTID, &stack.Node, &stack.Pool,
		&stack.Storage, &stack.Bridge, &stack.Cores, &stack.MemoryMB, &stack.DiskGB,
		&stack.Hostname, &stack.IPAddress, &stack.MACAddress, &onboot, &unprivileged, &stack.OSTemplate,
		&appsJSON, &mountsJSON, &devicesJSON, &envVarsJSON,
		&stack.Status, &createdAt,
	)
	if err != nil {
		return nil, err
	}
	stack.OnBoot = onboot != 0
	stack.Unprivileged = unprivileged != 0
	json.Unmarshal([]byte(appsJSON), &stack.Apps)
	json.Unmarshal([]byte(mountsJSON), &stack.MountPoints)
	json.Unmarshal([]byte(devicesJSON), &stack.Devices)
	json.Unmarshal([]byte(envVarsJSON), &stack.EnvVars)
	stack.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &stack, nil
}
