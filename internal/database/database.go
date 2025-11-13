package database

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// JobStatus는 작업 상태를 나타냅니다
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

// JobRecord는 작업 실행 기록을 나타냅니다
type JobRecord struct {
	ID          string    `json:"id"`
	Prompt      string    `json:"prompt"`
	ProjectPath string    `json:"project_path"`
	Status      JobStatus `json:"status"`
	Output      string    `json:"output,omitempty"`
	Error       string    `json:"error,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
	UserName    string    `json:"user_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Duration    int64     `json:"duration,omitempty"` // milliseconds
}

// DB는 SQLite 데이터베이스 연결을 관리합니다
type DB struct {
	conn *sql.DB
}

// NewDB는 새로운 데이터베이스 연결을 생성합니다
func NewDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// 연결 테스트
	if err := conn.Ping(); err != nil {
		return nil, err
	}

	db := &DB{conn: conn}

	// 테이블 초기화
	if err := db.initTables(); err != nil {
		return nil, err
	}

	log.Printf("✅ SQLite 데이터베이스 초기화 완료: %s", dbPath)
	return db, nil
}

// initTables는 필요한 테이블을 생성합니다
func (db *DB) initTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS job_records (
		id TEXT PRIMARY KEY,
		prompt TEXT NOT NULL,
		project_path TEXT,
		status TEXT NOT NULL,
		output TEXT,
		error TEXT,
		user_id TEXT,
		user_name TEXT,
		created_at DATETIME NOT NULL,
		started_at DATETIME,
		completed_at DATETIME,
		duration INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_job_status ON job_records(status);
	CREATE INDEX IF NOT EXISTS idx_job_created_at ON job_records(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_job_user_id ON job_records(user_id);
	`

	_, err := db.conn.Exec(schema)
	return err
}

// CreateJob은 새로운 작업 레코드를 생성합니다
func (db *DB) CreateJob(job *JobRecord) error {
	query := `
		INSERT INTO job_records (
			id, prompt, project_path, status, user_id, user_name, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.conn.Exec(query,
		job.ID,
		job.Prompt,
		job.ProjectPath,
		job.Status,
		job.UserID,
		job.UserName,
		job.CreatedAt,
	)

	return err
}

// UpdateJobStatus는 작업 상태를 업데이트합니다
func (db *DB) UpdateJobStatus(jobID string, status JobStatus) error {
	now := time.Now()
	var query string

	switch status {
	case JobStatusRunning:
		query = "UPDATE job_records SET status = ?, started_at = ? WHERE id = ?"
		_, err := db.conn.Exec(query, status, now, jobID)
		return err
	case JobStatusCompleted, JobStatusFailed:
		// duration 계산
		var startedAt *time.Time
		err := db.conn.QueryRow("SELECT started_at FROM job_records WHERE id = ?", jobID).Scan(&startedAt)
		if err != nil {
			return err
		}

		var duration int64
		if startedAt != nil {
			duration = now.Sub(*startedAt).Milliseconds()
		}

		query = "UPDATE job_records SET status = ?, completed_at = ?, duration = ? WHERE id = ?"
		_, err = db.conn.Exec(query, status, now, duration, jobID)
		return err
	default:
		query = "UPDATE job_records SET status = ? WHERE id = ?"
		_, err := db.conn.Exec(query, status, jobID)
		return err
	}
}

// UpdateJobResult는 작업 결과를 업데이트합니다
func (db *DB) UpdateJobResult(jobID string, output string, errMsg string) error {
	query := "UPDATE job_records SET output = ?, error = ? WHERE id = ?"
	_, err := db.conn.Exec(query, output, errMsg, jobID)
	return err
}

// GetJob은 작업 레코드를 조회합니다
func (db *DB) GetJob(jobID string) (*JobRecord, error) {
	query := `
		SELECT id, prompt, project_path, status, output, error, 
		       user_id, user_name, created_at, started_at, completed_at, duration
		FROM job_records WHERE id = ?
	`

	job := &JobRecord{}
	err := db.conn.QueryRow(query, jobID).Scan(
		&job.ID,
		&job.Prompt,
		&job.ProjectPath,
		&job.Status,
		&job.Output,
		&job.Error,
		&job.UserID,
		&job.UserName,
		&job.CreatedAt,
		&job.StartedAt,
		&job.CompletedAt,
		&job.Duration,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return job, nil
}

// ListJobs는 작업 목록을 조회합니다
func (db *DB) ListJobs(limit int, offset int, status JobStatus) ([]*JobRecord, error) {
	var query string
	var args []interface{}

	if status != "" {
		query = `
			SELECT id, prompt, project_path, status, output, error,
			       user_id, user_name, created_at, started_at, completed_at, duration
			FROM job_records
			WHERE status = ?
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`
		args = []interface{}{status, limit, offset}
	} else {
		query = `
			SELECT id, prompt, project_path, status, output, error,
			       user_id, user_name, created_at, started_at, completed_at, duration
			FROM job_records
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`
		args = []interface{}{limit, offset}
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*JobRecord
	for rows.Next() {
		job := &JobRecord{}
		err := rows.Scan(
			&job.ID,
			&job.Prompt,
			&job.ProjectPath,
			&job.Status,
			&job.Output,
			&job.Error,
			&job.UserID,
			&job.UserName,
			&job.CreatedAt,
			&job.StartedAt,
			&job.CompletedAt,
			&job.Duration,
		)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// Close는 데이터베이스 연결을 닫습니다
func (db *DB) Close() error {
	return db.conn.Close()
}

