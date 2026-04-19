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
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return &DatabaseService{}, err
	}

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

	return nil
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
