package logic

import (
	"context"

	"websocket_chat_go/backend/internal/model"
)

// Directory lists colleagues with optional department and job title metadata.
func (auth *Auth) Directory(ctx context.Context, userID string) ([]model.DirectoryEntry, error) {
	rows, err := auth.Store.DB.Query(ctx, `SELECT u.id,u.email,u.name,
		COALESCE(p.department,''),COALESCE(p.job_title,'')
		FROM users u LEFT JOIN user_profiles p ON p.user_id=u.id
		WHERE u.id<>$1 ORDER BY p.department,u.name,u.id LIMIT 1000`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []model.DirectoryEntry{}
	for rows.Next() {
		var entry model.DirectoryEntry
		if err := rows.Scan(&entry.ID, &entry.Email, &entry.Name, &entry.Department, &entry.JobTitle); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
