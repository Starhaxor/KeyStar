package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/starloader/backend/internal/domain"
)

// Console queries serve the admin dashboard. Every query is parameterized and
// read views never expose password hashes, HMACs or raw hardware identifiers.

func (s *Store) ConsoleOverview(ctx context.Context) (*domain.ConsoleOverview, error) {
	overview := &domain.ConsoleOverview{}
	err := s.db.QueryRow(ctx, `
		select
			(select count(*) from users),
			(select count(*) from licenses where status = 'active' and expires_at > now()),
			(select count(*) from devices where status = 'active'),
			(select count(*) from auth_sessions where status = 'verified' and expires_at > now())`).
		Scan(&overview.TotalUsers, &overview.ActiveLicenses, &overview.ActiveDevices, &overview.ActiveSessions)
	if err != nil {
		return nil, fmt.Errorf("console overview: %w", err)
	}
	recent, _, err := s.ListAuditLogs(ctx, 0, 8)
	if err != nil {
		return nil, err
	}
	overview.RecentAudit = recent
	return overview, nil
}

// ConsoleTodayStats aggregates the operations-center counters since UTC
// midnight in a single query. Expired licenses include both explicitly expired
// rows and active rows whose deadline has passed.
func (s *Store) ConsoleTodayStats(ctx context.Context) (*domain.ConsoleTodayStats, error) {
	var stats domain.ConsoleTodayStats
	err := s.db.QueryRow(ctx, `
		select
			(select count(*) from auth_sessions where created_at >= date_trunc('day', now())),
			(select count(*) from licenses where created_at >= date_trunc('day', now())),
			(select count(*) from devices where created_at >= date_trunc('day', now())),
			(select count(*) from audit_logs where action = 'ADMIN_LOGIN' and created_at >= date_trunc('day', now())),
			(select count(*) from audit_logs where action = 'ADMIN_LOGIN_FAILED' and created_at >= date_trunc('day', now())),
			(select count(*) from security_events where kind = 'ADMIN_PERMISSION_DENIED' and created_at >= date_trunc('day', now())),
			(select count(*) from users where status = 'banned'),
			(select count(*) from licenses where status = 'expired' or (status = 'active' and expires_at <= now()))`).
		Scan(&stats.LoginsToday, &stats.ActivationsToday, &stats.NewDevicesToday,
			&stats.AdminLoginsToday, &stats.FailedLoginsToday, &stats.PermissionDeniedToday,
			&stats.BannedUsers, &stats.ExpiredLicenses)
	if err != nil {
		return nil, fmt.Errorf("console today stats: %w", err)
	}
	return &stats, nil
}

// ConsoleDailyStats returns a per-day activity series (licenses created,
// devices registered, sessions created, audit events and admin logins) for
// the trailing days window. Days without events are included with zeroes.
func (s *Store) ConsoleDailyStats(ctx context.Context, days int) ([]domain.DailyStat, error) {
	if days < 1 || days > 90 {
		days = 14
	}
	countByDay := func(query string, args ...any) (map[string]int64, error) {
		rows, err := s.db.Query(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		counts := make(map[string]int64)
		for rows.Next() {
			var day string
			var count int64
			if err := rows.Scan(&day, &count); err != nil {
				return nil, err
			}
			counts[day] = count
		}
		return counts, rows.Err()
	}
	window := fmt.Sprintf("created_at >= now() - interval '%d days'", days)
	licenses, err := countByDay(`select to_char(created_at at time zone 'UTC', 'YYYY-MM-DD'), count(*) from licenses where ` + window + ` group by 1`)
	if err != nil {
		return nil, fmt.Errorf("daily licenses: %w", err)
	}
	devices, err := countByDay(`select to_char(created_at at time zone 'UTC', 'YYYY-MM-DD'), count(*) from devices where ` + window + ` group by 1`)
	if err != nil {
		return nil, fmt.Errorf("daily devices: %w", err)
	}
	sessions, err := countByDay(`select to_char(created_at at time zone 'UTC', 'YYYY-MM-DD'), count(*) from auth_sessions where ` + window + ` group by 1`)
	if err != nil {
		return nil, fmt.Errorf("daily sessions: %w", err)
	}
	audit, err := countByDay(`select to_char(created_at at time zone 'UTC', 'YYYY-MM-DD'), count(*) from audit_logs where ` + window + ` group by 1`)
	if err != nil {
		return nil, fmt.Errorf("daily audit: %w", err)
	}
	logins, err := countByDay(`select to_char(created_at at time zone 'UTC', 'YYYY-MM-DD'), count(*) from audit_logs where action = 'ADMIN_LOGIN' and ` + window + ` group by 1`)
	if err != nil {
		return nil, fmt.Errorf("daily logins: %w", err)
	}

	start := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))
	stats := make([]domain.DailyStat, 0, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		stats = append(stats, domain.DailyStat{
			Day:               day,
			LicensesCreated:   licenses[day],
			DevicesRegistered: devices[day],
			SessionsCreated:   sessions[day],
			AuditEvents:       audit[day],
			AdminLogins:       logins[day],
		})
	}
	return stats, nil
}

func (s *Store) ListConsoleUsers(ctx context.Context, applicationID string, offset, limit int, search string, status string) ([]domain.ConsoleUser, int64, error) {
	search = strings.ToLower(strings.TrimSpace(search))
	status = strings.ToLower(strings.TrimSpace(status))
	var total int64
	countQuery := `select count(*) from users`
	countArgs := []any{applicationID}
	where := []string{"application_id = $1::uuid"}
	if search != "" {
		where = append(where, "position($2 in email) > 0")
		countArgs = append(countArgs, search)
	}
	if status == "active" || status == "disabled" || status == "banned" {
		where = append(where, fmt.Sprintf("status = '%s'", status))
	}
	if len(where) > 0 {
		countQuery = "select count(*) from users where " + strings.Join(where, " and ")
	}
	if err := s.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count console users: %w", err)
	}
	rows, err := s.db.Query(ctx, `
		with filtered as (
			select id
			from users
			where application_id = $1::uuid
			  and ($4 = '' or position($4 in email) > 0)
			  and ($5 = '' or status = $5)
			order by created_at desc, id desc
			limit $2 offset $1
		)
		select u.id::text, u.email, u.status, u.created_at,
			(select count(*)::integer from licenses l where l.user_id = u.id),
			(select count(*)::integer from devices d where d.user_id = u.id),
			(select count(*)::integer from auth_sessions ss
				where ss.user_id = u.id and ss.status = 'verified' and ss.expires_at > now()),
			(select max(ss.created_at) from auth_sessions ss where ss.user_id = u.id)
		from filtered f
		join users u on u.id = f.id
		order by u.created_at desc, u.id desc`, applicationID, offset, limit, search, status)
	if err != nil {
		return nil, 0, fmt.Errorf("list console users: %w", err)
	}
	defer rows.Close()
	users := make([]domain.ConsoleUser, 0, limit)
	for rows.Next() {
		var user domain.ConsoleUser
		if err := rows.Scan(&user.ID, &user.Email, &user.Status, &user.CreatedAt,
			&user.LicenseCount, &user.DeviceCount, &user.ActiveSessionCount, &user.LastLoginAt); err != nil {
			return nil, 0, fmt.Errorf("scan console user: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list console users: %w", err)
	}
	return users, total, nil
}

func (s *Store) ConsoleUserDetail(ctx context.Context, applicationID, userID string) (*domain.ConsoleUserDetail, error) {
	row := s.db.QueryRow(ctx, `
		select u.id::text, u.email, u.status, u.created_at,
			(select count(*)::integer from licenses l where l.user_id = u.id),
			(select count(*)::integer from devices d where d.user_id = u.id),
			(select count(*)::integer from auth_sessions ss
				where ss.user_id = u.id and ss.status = 'verified' and ss.expires_at > now()),
			(select max(ss.created_at) from auth_sessions ss where ss.user_id = u.id),
			u.notes, u.ban_reason, u.banned_at, u.ban_expires_at
		from users u
		where u.id = $1::uuid and u.application_id = $2::uuid`, userID, applicationID)
	var user domain.ConsoleUser
	detail := &domain.ConsoleUserDetail{}
	err := row.Scan(&user.ID, &user.Email, &user.Status, &user.CreatedAt,
		&user.LicenseCount, &user.DeviceCount, &user.ActiveSessionCount, &user.LastLoginAt,
		&detail.Notes, &detail.BanReason, &detail.BannedAt, &detail.BanExpiresAt)
	detail.User = user
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("find console user: %w", err)
	}
	if detail.Licenses, err = s.listConsoleLicenses(ctx, "where l.user_id = $1", userID); err != nil {
		return nil, err
	}
	if detail.Devices, err = s.listConsoleDevices(ctx, "where d.user_id = $1", userID); err != nil {
		return nil, err
	}
	if detail.Sessions, err = s.listConsoleSessions(ctx, "where ss.user_id = $1", userID); err != nil {
		return nil, err
	}
	return detail, nil
}

func (s *Store) SetUserStatus(ctx context.Context, applicationID, userID string, status domain.UserStatus) error {
	err := s.db.QueryRow(ctx, `
		update users
		set status = $2, updated_at = now()
		where id = $1::uuid and application_id = $3::uuid
		returning id`, userID, string(status), applicationID).Scan(new(string))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUserNotFound
		}
		return fmt.Errorf("set user status: %w", err)
	}
	return nil
}

// SetUserNotes stores free-form admin notes about a user (KeyAuth-style
// "note" field).
func (s *Store) SetUserNotes(ctx context.Context, applicationID, userID, notes string) error {
	err := s.db.QueryRow(ctx, `
		update users
		set notes = $2, updated_at = now()
		where id = $1::uuid and application_id = $3::uuid
		returning id`, userID, notes, applicationID).Scan(new(string))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUserNotFound
		}
		return fmt.Errorf("set user notes: %w", err)
	}
	return nil
}

// BanUser bans a user with a reason and records the ban in the bans table.
// When expiresAt is non-nil the ban is temporary and the account reopens
// automatically once the timestamp passes (see AutoUnbanExpired). The client
// login path rejects any non-active user, so the ban takes effect immediately.
func (s *Store) BanUser(ctx context.Context, applicationID, userID, reason string, expiresAt *time.Time) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin ban user: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	err = tx.QueryRow(ctx, `
		update users
		set status = 'banned', ban_reason = $2, banned_at = now(),
		    ban_expires_at = $3, updated_at = now()
		where id = $1::uuid and application_id = $4::uuid
		returning id`, userID, reason, expiresAt, applicationID).Scan(new(string))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUserNotFound
		}
		return fmt.Errorf("ban user: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		update bans
		set status = 'lifted', lifted_at = now(), lift_reason = 'superseded'
		where user_id = $1::uuid and status = 'active'`, userID); err != nil {
		return fmt.Errorf("supersede previous ban record: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		insert into bans (user_id, reason, expires_at)
		values ($1::uuid, $2, $3)`, userID, reason, expiresAt); err != nil {
		return fmt.Errorf("record ban: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit ban user: %w", err)
	}
	return nil
}

// AutoUnbanExpired reopens a user whose temporary ban has expired and marks
// the active ban record as expired. It is a no-op unless the user is banned
// and the ban deadline has passed.
func (s *Store) AutoUnbanExpired(ctx context.Context, applicationID, userID string) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin auto unban: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	err = tx.QueryRow(ctx, `
		update users
		set status = 'active', ban_reason = '', banned_at = null, ban_expires_at = null,
		    updated_at = now()
		where id = $1::uuid and application_id = $2::uuid and status = 'banned'
		  and ban_expires_at is not null and ban_expires_at <= now()
		returning id`, userID, applicationID).Scan(new(string))
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("auto unban expired user: %w", err)
		}
		return nil
	}
	if _, err := tx.Exec(ctx, `
		update bans
		set status = 'expired', lifted_at = now(), lift_reason = 'expired'
		where user_id = $1::uuid and status = 'active'
		  and expires_at is not null and expires_at <= now()`, userID); err != nil {
		return fmt.Errorf("expire ban record: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit auto unban: %w", err)
	}
	return nil
}

// UnbanUser clears a ban, restores the user to active, and lifts the active
// ban record.
func (s *Store) UnbanUser(ctx context.Context, applicationID, userID string) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin unban user: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	err = tx.QueryRow(ctx, `
		update users
		set status = 'active', ban_reason = '', banned_at = null, ban_expires_at = null,
		    updated_at = now()
		where id = $1::uuid and application_id = $2::uuid
		returning id`, userID, applicationID).Scan(new(string))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrUserNotFound
		}
		return fmt.Errorf("unban user: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		update bans
		set status = 'lifted', lifted_at = now(), lift_reason = 'admin'
		where user_id = $1::uuid and status = 'active'`, userID); err != nil {
		return fmt.Errorf("lift ban record: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit unban user: %w", err)
	}
	return nil
}

// ListConsoleBans pages the ban history. statusFilter limits rows to one ban
// state (active, lifted, expired); search is a substring match on the user's
// email.
func (s *Store) ListConsoleBans(ctx context.Context, offset, limit int, search, statusFilter string) ([]domain.BanRecord, int64, error) {
	search = strings.ToLower(strings.TrimSpace(search))
	statusFilter = strings.ToLower(strings.TrimSpace(statusFilter))
	where := []string{}
	args := []any{}
	if search != "" {
		args = append(args, search)
		where = append(where, fmt.Sprintf("position($%d in lower(u.email)) > 0", len(args)))
	}
	if statusFilter == "active" || statusFilter == "lifted" || statusFilter == "expired" {
		args = append(args, statusFilter)
		where = append(where, fmt.Sprintf("b.status = $%d", len(args)))
	}
	whereClause := ""
	if len(where) > 0 {
		whereClause = "where " + strings.Join(where, " and ")
	}
	var total int64
	if err := s.db.QueryRow(ctx, `
		select count(*) from bans b join users u on u.id = b.user_id `+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count console bans: %w", err)
	}
	args = append(args, limit, offset)
	rows, err := s.db.Query(ctx, `
		select b.id::text, b.user_id::text, u.email, b.reason, b.expires_at, b.status,
			b.banned_at, b.lifted_at, b.lift_reason
		from bans b
		join users u on u.id = b.user_id
		`+whereClause+`
		order by b.banned_at desc, b.id desc
		limit $`+strconv.Itoa(len(args)-1)+` offset $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list console bans: %w", err)
	}
	defer rows.Close()
	bans := make([]domain.BanRecord, 0, limit)
	for rows.Next() {
		var ban domain.BanRecord
		if err := rows.Scan(&ban.ID, &ban.UserID, &ban.UserEmail, &ban.Reason, &ban.ExpiresAt,
			&ban.Status, &ban.BannedAt, &ban.LiftedAt, &ban.LiftReason); err != nil {
			return nil, 0, fmt.Errorf("scan console ban: %w", err)
		}
		bans = append(bans, ban)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list console bans: %w", err)
	}
	return bans, total, nil
}

// ResetUserDevices revokes every device record of a user within one
// application, forcing the client to re-register its hardware
// (KeyAuth-style HWID reset).
func (s *Store) ResetUserDevices(ctx context.Context, applicationID, userID string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin user device reset: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `delete from devices where user_id = $1::uuid and application_id = $2::uuid`, userID, applicationID)
	if err != nil {
		return 0, fmt.Errorf("reset user devices: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit user device reset: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *Store) ListConsoleLicenses(ctx context.Context, applicationID string, offset, limit int) ([]domain.ConsoleLicense, int64, error) {
	var total int64
	if err := s.db.QueryRow(ctx, `select count(*) from licenses where application_id = $1::uuid`, applicationID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count console licenses: %w", err)
	}
	licenses, err := s.listConsoleLicenses(ctx, "where l.application_id = $1::uuid order by l.created_at desc, l.id desc limit $2 offset $3", applicationID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return licenses, total, nil
}

func (s *Store) listConsoleLicenses(ctx context.Context, tail string, args ...any) ([]domain.ConsoleLicense, error) {
	rows, err := s.db.Query(ctx, `
		select l.id::text, l.user_id::text, u.email, l.product_id::text, l.plan_id::text, p.name, l.status, l.level, l.max_devices, l.notes, l.expires_at, l.created_at
		from licenses l
		join users u on u.id = l.user_id
		join products p on p.id = l.product_id
		`+tail, args...)
	if err != nil {
		return nil, fmt.Errorf("list console licenses: %w", err)
	}
	defer rows.Close()
	licenses := make([]domain.ConsoleLicense, 0)
	for rows.Next() {
		var license domain.ConsoleLicense
		if err := rows.Scan(&license.ID, &license.UserID, &license.UserEmail,
			&license.ProductID, &license.PlanID, &license.Product,
			&license.Status, &license.Level, &license.MaxDevices, &license.Notes, &license.ExpiresAt, &license.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan console license: %w", err)
		}
		licenses = append(licenses, license)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list console licenses: %w", err)
	}
	return licenses, nil
}

func (s *Store) FindLicenseByID(ctx context.Context, applicationID, licenseID string) (*domain.License, error) {
	license, err := scanLicense(s.db.QueryRow(ctx,
		`select `+licenseColumns+` from licenses l
		 join products p on p.id = l.product_id
		 where l.id = $1::uuid and l.application_id = $2::uuid`, licenseID, applicationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrLicenseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find license by id: %w", err)
	}
	return license, nil
}

// AdminUpdateLicense extends expiry, adjusts the device limit and updates the
// KeyAuth-style level and notes. Revoked licenses cannot be modified; a
// renewed license returns to active status.
func (s *Store) AdminUpdateLicense(ctx context.Context, applicationID, licenseID string, expiresAt time.Time, maxDevices, level int, notes string) error {
	err := s.db.QueryRow(ctx, `
		update licenses
		set expires_at = $2, max_devices = $3, level = $4, notes = $5, status = 'active', updated_at = now()
		where id = $1::uuid and application_id = $6::uuid and status <> 'revoked'
		returning id`, licenseID, expiresAt, maxDevices, level, notes, applicationID).Scan(new(string))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrLicenseNotFound
		}
		return fmt.Errorf("update license: %w", err)
	}
	return nil
}

// --- Variables (KeyAuth-style key-value store) ---

func (s *Store) ListVariables(ctx context.Context, applicationID string) ([]domain.Variable, error) {
	rows, err := s.db.Query(ctx, `
		select id::text, key, value, description, created_at, updated_at
		from variables
		where application_id = $1::uuid
		order by key asc`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list variables: %w", err)
	}
	defer rows.Close()
	variables := make([]domain.Variable, 0)
	for rows.Next() {
		var variable domain.Variable
		if err := rows.Scan(&variable.ID, &variable.Key, &variable.Value, &variable.Description, &variable.CreatedAt, &variable.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan variable: %w", err)
		}
		variables = append(variables, variable)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list variables: %w", err)
	}
	return variables, nil
}

func (s *Store) CreateVariable(ctx context.Context, applicationID, key, value, description string) (*domain.Variable, error) {
	row := s.db.QueryRow(ctx, `
		insert into variables (application_id, key, value, description)
		values ($1, $2, $3, $4)
		returning id::text, key, value, description, created_at, updated_at`,
		applicationID, key, value, description)
	var variable domain.Variable
	if err := row.Scan(&variable.ID, &variable.Key, &variable.Value, &variable.Description, &variable.CreatedAt, &variable.UpdatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrVariableAlreadyExists
		}
		return nil, fmt.Errorf("create variable: %w", err)
	}
	return &variable, nil
}

func (s *Store) UpdateVariable(ctx context.Context, applicationID, variableID, value, description string) error {
	err := s.db.QueryRow(ctx, `
		update variables
		set value = $2, description = $3, updated_at = now()
		where id = $1::uuid and application_id = $4::uuid
		returning id`, variableID, value, description, applicationID).Scan(new(string))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrVariableNotFound
		}
		return fmt.Errorf("update variable: %w", err)
	}
	return nil
}

func (s *Store) DeleteVariable(ctx context.Context, applicationID, variableID string) error {
	err := s.db.QueryRow(ctx, `
		delete from variables
		where id = $1::uuid and application_id = $2::uuid
		returning id`, variableID, applicationID).Scan(new(string))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrVariableNotFound
		}
		return fmt.Errorf("delete variable: %w", err)
	}
	return nil
}

func (s *Store) AdminRevokeLicense(ctx context.Context, applicationID, licenseID string) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin license revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `
		update licenses
		set status = 'revoked', updated_at = now()
		where id = $1::uuid and application_id = $2::uuid and status <> 'revoked'`, licenseID, applicationID)
	if err != nil {
		return fmt.Errorf("revoke license: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrLicenseNotFound
	}
	if _, err := tx.Exec(ctx, `
		update auth_sessions
		set status = 'expired', updated_at = now()
		where license_id = $1::uuid and application_id = $2::uuid and status in ('pending', 'verified')`, licenseID, applicationID); err != nil {
		return fmt.Errorf("expire revoked license sessions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit license revocation: %w", err)
	}
	return nil
}

func (s *Store) ListConsoleDevices(ctx context.Context, applicationID string, offset, limit int) ([]domain.ConsoleDevice, int64, error) {
	var total int64
	if err := s.db.QueryRow(ctx, `select count(*) from devices where application_id = $1::uuid`, applicationID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count console devices: %w", err)
	}
	devices, err := s.listConsoleDevices(ctx, "where d.application_id = $1::uuid order by d.last_seen_at desc, d.id desc limit $2 offset $3", applicationID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return devices, total, nil
}

func (s *Store) listConsoleDevices(ctx context.Context, tail string, args ...any) ([]domain.ConsoleDevice, error) {
	rows, err := s.db.Query(ctx, `
		select d.id::text, d.user_id::text, u.email, d.license_id::text,
			octet_length(d.tpm_public_key) > 0,
			d.smbios_uuid_hmac is not null,
			d.motherboard_serial_hmac is not null,
			d.bios_serial_hmac is not null,
			d.system_disk_serial_hmac is not null,
			d.machine_guid_hmac is not null,
			d.status, d.created_at, d.last_seen_at
		from devices d
		join users u on u.id = d.user_id
		`+tail, args...)
	if err != nil {
		return nil, fmt.Errorf("list console devices: %w", err)
	}
	defer rows.Close()
	devices := make([]domain.ConsoleDevice, 0)
	for rows.Next() {
		var device domain.ConsoleDevice
		if err := rows.Scan(&device.ID, &device.UserID, &device.UserEmail, &device.LicenseID,
			&device.TPMRegistered, &device.HasSMBIOSUUID, &device.HasMotherboardSerial,
			&device.HasBIOSSerial, &device.HasSystemDiskSerial, &device.HasMachineGUID,
			&device.Status, &device.CreatedAt, &device.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan console device: %w", err)
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list console devices: %w", err)
	}
	return devices, nil
}

// FindConsoleDeviceByID returns the redacted device view plus the TPM public
// key fingerprint and the license product. No raw hardware identifier leaves
// the database.
func (s *Store) FindConsoleDeviceByID(ctx context.Context, applicationID, deviceID string) (*domain.ConsoleDeviceDetail, error) {
	row := s.db.QueryRow(ctx, `
		select d.id::text, d.user_id::text, u.email, d.license_id::text,
			octet_length(d.tpm_public_key) > 0,
			d.smbios_uuid_hmac is not null,
			d.motherboard_serial_hmac is not null,
			d.bios_serial_hmac is not null,
			d.system_disk_serial_hmac is not null,
			d.machine_guid_hmac is not null,
			d.status, d.created_at, d.last_seen_at,
			p.name, encode(d.tpm_public_key_sha256, 'hex')
		from devices d
		join users u on u.id = d.user_id
		join licenses l on l.id = d.license_id
		join products p on p.id = l.product_id
		where d.id = $1::uuid and d.application_id = $2::uuid`, deviceID, applicationID)
	var detail domain.ConsoleDeviceDetail
	err := row.Scan(&detail.Device.ID, &detail.Device.UserID, &detail.Device.UserEmail, &detail.Device.LicenseID,
		&detail.Device.TPMRegistered, &detail.Device.HasSMBIOSUUID, &detail.Device.HasMotherboardSerial,
		&detail.Device.HasBIOSSerial, &detail.Device.HasSystemDiskSerial, &detail.Device.HasMachineGUID,
		&detail.Device.Status, &detail.Device.CreatedAt, &detail.Device.LastSeenAt,
		&detail.Product, &detail.TPMFingerprint)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDeviceNotFound
		}
		return nil, fmt.Errorf("find console device: %w", err)
	}
	return &detail, nil
}

// AdminResetDevice removes the hardware registration entirely so the user can
// register a fresh device; pending sessions bound to the license stay intact.
func (s *Store) AdminResetDevice(ctx context.Context, applicationID, deviceID string) error {
	err := s.db.QueryRow(ctx, `
		delete from devices
		where id = $1::uuid and application_id = $2::uuid
		returning id`, deviceID, applicationID).Scan(new(string))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrDeviceNotFound
		}
		return fmt.Errorf("reset device: %w", err)
	}
	return nil
}

// RevokeUserSessions expires every pending or verified auth session of the
// user and reports how many were revoked.
// BulkSetUserStatus enables or disables several end-user accounts in one
// statement and returns how many rows changed.
func (s *Store) BulkSetUserStatus(ctx context.Context, applicationID string, userIDs []string, status domain.UserStatus) (int64, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin bulk user status: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `
		update users
		set status = $2, updated_at = now()
		where id = any($1::uuid[]) and application_id = $3::uuid`, userIDs, string(status), applicationID)
	if err != nil {
		return 0, fmt.Errorf("bulk set user status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit bulk user status: %w", err)
	}
	return tag.RowsAffected(), nil
}

// BulkRevokeUserSessions expires every pending or verified auth session of
// several users in one statement.
func (s *Store) BulkRevokeUserSessions(ctx context.Context, applicationID string, userIDs []string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin bulk user session revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `
		update auth_sessions
		set status = 'expired', updated_at = now()
		where user_id = any($1::uuid[]) and application_id = $2::uuid and status in ('pending', 'verified')`, userIDs, applicationID)
	if err != nil {
		return 0, fmt.Errorf("bulk revoke user sessions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit bulk user session revocation: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *Store) RevokeUserSessions(ctx context.Context, applicationID, userID string) (int64, error) {
	var exists bool
	if err := s.db.QueryRow(ctx, `select exists(select 1 from users where id = $1::uuid and application_id = $2::uuid)`, userID, applicationID).Scan(&exists); err != nil {
		return 0, fmt.Errorf("check user for session revocation: %w", err)
	}
	if !exists {
		return 0, domain.ErrUserNotFound
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin user session revocation: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	tag, err := tx.Exec(ctx, `
		update auth_sessions
		set status = 'expired', updated_at = now()
		where user_id = $1::uuid and application_id = $2::uuid and status in ('pending', 'verified')`, userID, applicationID)
	if err != nil {
		return 0, fmt.Errorf("revoke user sessions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit user session revocation: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *Store) AdminRevokeDevice(ctx context.Context, applicationID, deviceID string) error {
	err := s.db.QueryRow(ctx, `
		update devices
		set status = 'revoked', updated_at = now()
		where id = $1::uuid and application_id = $2::uuid and status = 'active'
		returning id`, deviceID, applicationID).Scan(new(string))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrDeviceNotFound
		}
		return fmt.Errorf("revoke device: %w", err)
	}
	return nil
}

func (s *Store) ListConsoleSessions(ctx context.Context, applicationID string, offset, limit int) ([]domain.ConsoleSession, int64, error) {
	var total int64
	if err := s.db.QueryRow(ctx, `select count(*) from auth_sessions where application_id = $1::uuid`, applicationID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count console sessions: %w", err)
	}
	sessions, err := s.listConsoleSessions(ctx, "where ss.application_id = $1::uuid order by ss.created_at desc, ss.id desc limit $2 offset $3", applicationID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return sessions, total, nil
}

func (s *Store) listConsoleSessions(ctx context.Context, tail string, args ...any) ([]domain.ConsoleSession, error) {
	rows, err := s.db.Query(ctx, `
		select ss.id::text, ss.user_id::text, u.email, ss.license_id::text, ss.status, ss.expires_at, ss.created_at
		from auth_sessions ss
		join users u on u.id = ss.user_id
		`+tail, args...)
	if err != nil {
		return nil, fmt.Errorf("list console sessions: %w", err)
	}
	defer rows.Close()
	sessions := make([]domain.ConsoleSession, 0)
	for rows.Next() {
		var session domain.ConsoleSession
		if err := rows.Scan(&session.ID, &session.UserID, &session.UserEmail, &session.LicenseID,
			&session.Status, &session.ExpiresAt, &session.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan console session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list console sessions: %w", err)
	}
	return sessions, nil
}

func (s *Store) AdminRevokeAuthSession(ctx context.Context, applicationID, sessionID string) error {
	err := s.db.QueryRow(ctx, `
		update auth_sessions
		set status = 'expired', updated_at = now()
		where id = $1::uuid and application_id = $2::uuid and status in ('pending', 'verified')
		returning id`, sessionID, applicationID).Scan(new(string))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrAuthSessionNotFound
		}
		return fmt.Errorf("revoke auth session: %w", err)
	}
	return nil
}
