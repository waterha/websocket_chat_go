package logic

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"websocket_chat_go/backend/internal/model"
)

// Directory searches other accounts on this server. Self-maintained profiles do
// not grant organizational roles.
func (auth *Auth) Directory(ctx context.Context, userID, query, department string, offset, limit int) (model.DirectoryPage, error) {
	page := model.DirectoryPage{Entries: []model.DirectoryEntry{}, Departments: []string{}}
	query, department = strings.TrimSpace(query), strings.TrimSpace(department)
	if offset < 0 || offset > 1000000 || limit < 1 || limit > 100 || utf8.RuneCountInString(query) > 200 || utf8.RuneCountInString(department) > 100 {
		return page, model.Fail(400, "invalid_directory_query")
	}
	rows, err := auth.Store.DB.Query(ctx, `SELECT u.id,u.email,u.name,
		COALESCE(p.department,''),COALESCE(p.job_title,'')
		FROM users u LEFT JOIN user_profiles p ON p.user_id=u.id
		WHERE u.id<>$1 AND ($2='' OR
		strpos(lower(u.name),lower($2))>0 OR strpos(lower(u.email),lower($2))>0 OR
		strpos(lower(COALESCE(p.department,'')),lower($2))>0 OR strpos(lower(COALESCE(p.job_title,'')),lower($2))>0)
		AND ($3='' OR p.department=$3)
		ORDER BY COALESCE(p.department,''),u.name,u.id LIMIT $4 OFFSET $5`, userID, query, department, limit+1, offset)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var entry model.DirectoryEntry
		if err := rows.Scan(&entry.ID, &entry.Email, &entry.Name, &entry.Department, &entry.JobTitle); err != nil {
			rows.Close()
			return page, err
		}
		page.Entries = append(page.Entries, entry)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return page, err
	}
	page.HasMore = len(page.Entries) > limit
	if page.HasMore {
		page.Entries = page.Entries[:limit]
	}
	page.NextOffset = offset + len(page.Entries)
	departments, err := auth.Store.DB.Query(ctx, `SELECT DISTINCT department FROM user_profiles WHERE user_id<>$1 AND department<>'' ORDER BY department`, userID)
	if err != nil {
		return page, err
	}
	defer departments.Close()
	for departments.Next() {
		var value string
		if err := departments.Scan(&value); err != nil {
			return page, err
		}
		page.Departments = append(page.Departments, value)
	}
	return page, departments.Err()
}

func (auth *Auth) WorkProfile(ctx context.Context, userID string) (model.WorkProfile, error) {
	var profile model.WorkProfile
	err := auth.Store.DB.QueryRow(ctx, `SELECT COALESCE(p.department,''),COALESCE(p.job_title,'')
		FROM users u LEFT JOIN user_profiles p ON p.user_id=u.id WHERE u.id=$1`, userID).Scan(&profile.Department, &profile.JobTitle)
	return profile, err
}

func (auth *Auth) UpdateWorkProfile(ctx context.Context, userID string, profile model.WorkProfile) (model.WorkProfile, error) {
	profile.Department = strings.TrimSpace(profile.Department)
	profile.JobTitle = strings.TrimSpace(profile.JobTitle)
	for _, field := range []string{profile.Department, profile.JobTitle} {
		if utf8.RuneCountInString(field) > 100 || strings.ContainsFunc(field, unicode.IsControl) {
			return profile, model.Fail(400, "invalid_work_profile")
		}
	}
	_, err := auth.Store.DB.Exec(ctx, `INSERT INTO user_profiles(user_id,department,job_title) VALUES($1,$2,$3)
		ON CONFLICT(user_id) DO UPDATE SET department=excluded.department,job_title=excluded.job_title,updated_at=now()`, userID, profile.Department, profile.JobTitle)
	return profile, err
}
