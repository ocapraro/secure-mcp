package database

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type DatabaseService struct {
	path string
	db   *sql.DB
}

func NewDatabaseService(path string) (*DatabaseService, error) {
	// Enable WAL + busy timeout to reduce writer contention under concurrent PATCH calls.
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return &DatabaseService{}, err
	}

	// Keep a small connection pool; SQLite still has a single writer but benefits from pooled readers.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := db.Ping(); err != nil {
		return &DatabaseService{}, err
	}

	return &DatabaseService{
		path: path,
		db:   db,
	}, nil
}

func (s *DatabaseService) Close() {
	s.db.Close()
}

func (s *DatabaseService) Init() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			model TEXT NOT NULL
		);
	`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role TEXT NOT NULL CHECK(role IN ('user', 'assistant', 'system')),
			content TEXT NOT NULL,
			session_id INTEGER NOT NULL,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);
	`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		CREATE TABLE IF NOT EXISTS specialist_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			specialist TEXT NOT NULL,
			task TEXT NOT NULL,
			script TEXT NOT NULL,
			ok INTEGER NOT NULL DEFAULT 0,
			output TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT ''
		);
	`)
	if err != nil {
		return err
	}

	return nil
}

func (s *DatabaseService) InsertSpecialistLog(log CreateSpecialistLog) error {
	okInt := 0
	if log.OK {
		okInt = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO specialist_logs (specialist, task, script, ok, output, error)
		VALUES (?, ?, ?, ?, ?, ?)
	`, log.Specialist, log.Task, log.Script, okInt, log.Output, log.Error)
	return err
}

func (s *DatabaseService) GetSpecialistLogs(specialist string) ([]SpecialistLog, error) {
	var rows *sql.Rows
	var err error
	if specialist == "" {
		rows, err = s.db.Query(`
			SELECT id, created_at, specialist, task, script, ok, output, error
			FROM specialist_logs
			ORDER BY id DESC
			LIMIT 500
		`)
	} else {
		rows, err = s.db.Query(`
			SELECT id, created_at, specialist, task, script, ok, output, error
			FROM specialist_logs
			WHERE specialist = ?
			ORDER BY id DESC
			LIMIT 500
		`, specialist)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []SpecialistLog
	for rows.Next() {
		var l SpecialistLog
		var okInt int
		if err := rows.Scan(&l.ID, &l.CreatedAt, &l.Specialist, &l.Task, &l.Script, &okInt, &l.Output, &l.Error); err != nil {
			return nil, err
		}
		l.OK = okInt != 0
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (s *DatabaseService) GetSessions() ([]PartialSession, error) {
	rows, err := s.db.Query(`
		SELECT id, title, updated_at, model
		FROM sessions
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []PartialSession
	for rows.Next() {
		var session PartialSession

		if err := rows.Scan(
			&session.ID,
			&session.Title,
			&session.UpdatedAt,
			&session.Model,
		); err != nil {
			return nil, err
		}

		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}

func (s *DatabaseService) CreateSession(session CreateSession) (int64, error) {
	result, err := s.db.Exec(`
		INSERT INTO sessions (title, model)
		VALUES (?, ?)
	`, session.Title, session.Model)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (s *DatabaseService) GetSessionByID(id int64) (Session, error) {
	var session Session

	err := s.db.QueryRow(`
		SELECT id, title, updated_at, model
		FROM sessions
		WHERE id = ?
	`, id).Scan(
		&session.ID,
		&session.Title,
		&session.UpdatedAt,
		&session.Model,
	)
	if err != nil {
		return Session{}, err
	}

	rows, err := s.db.Query(`
		SELECT id, role, content, session_id
		FROM messages
		WHERE session_id = ?
		ORDER BY id ASC
	`, id)
	if err != nil {
		return Session{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.ID, &message.Role, &message.Content, &message.SessionID); err != nil {
			return Session{}, err
		}
		session.Messages = append(session.Messages, message)
	}
	if err := rows.Err(); err != nil {
		return Session{}, err
	}

	return session, nil
}

func (s *DatabaseService) UpdateSessionByID(id int64, update UpdateSession) (Session, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Session{}, err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE sessions
		SET model = ?,
		    title = CASE WHEN ? != '' THEN ? ELSE title END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, update.Model, update.Title, update.Title, id)
	if err != nil {
		return Session{}, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Session{}, err
	}
	if rowsAffected == 0 {
		return Session{}, sql.ErrNoRows
	}

	if _, err := tx.Exec(`
		DELETE FROM messages
		WHERE session_id = ?
	`, id); err != nil {
		return Session{}, err
	}

	for _, msg := range update.Messages {
		if _, err := tx.Exec(`
			INSERT INTO messages (role, content, session_id)
			VALUES (?, ?, ?)
		`, msg.Role, msg.Content, id); err != nil {
			return Session{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Session{}, err
	}

	return s.GetSessionByID(id)
}

func (s *DatabaseService) DeleteSessionByID(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		DELETE FROM messages
		WHERE session_id = ?
	`, id); err != nil {
		return err
	}

	result, err := tx.Exec(`
		DELETE FROM sessions
		WHERE id = ?
	`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return tx.Commit()
}
