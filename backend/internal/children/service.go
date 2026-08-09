package children

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound           = errors.New("child not found")
	ErrNicknameExists     = errors.New("active nickname already exists")
	ErrIdempotency        = errors.New("idempotency key reused with different input")
	ErrPINRequired        = errors.New("child pin required")
	ErrInvalidPIN         = errors.New("invalid child pin")
	ErrProfileUnavailable = errors.New("child profile or credential unavailable")
	ErrParentAuthority    = errors.New("parent authority is no longer active")
)

type Child struct {
	ID, Nickname, Avatar, Color string
	PINEnabled, Active          bool
	CreatedAt, UpdatedAt        time.Time
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func scanChild(row pgx.Row) (Child, error) {
	var c Child
	var archived *time.Time
	err := row.Scan(&c.ID, &c.Nickname, &c.Avatar, &c.Color, &c.PINEnabled, &archived, &c.CreatedAt, &c.UpdatedAt)
	c.Active = archived == nil
	return c, err
}

func (s *Service) List(ctx context.Context, familyID string, includeArchived bool) ([]Child, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,nickname,avatar,color,pin_hash IS NOT NULL,archived_at,created_at,updated_at FROM children WHERE family_id=$1 AND ($2 OR archived_at IS NULL) ORDER BY archived_at NULLS FIRST, lower(nickname), id`, familyID, includeArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Child, 0)
	for rows.Next() {
		c, err := scanChild(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Service) Create(ctx context.Context, sessionID, userID, familyID, key string, requestHash []byte, nickname, avatar, color, pinHash string) (Child, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Child{}, false, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return Child{}, false, err
	}
	var existingHash []byte
	var body []byte
	err = tx.QueryRow(ctx, `SELECT request_hash,response_body FROM idempotency_records WHERE family_id=$1 AND session_id=$2 AND route_family='children.create' AND idempotency_key=$3 FOR UPDATE`, familyID, sessionID, key).Scan(&existingHash, &body)
	if err == nil {
		if !equalHash(existingHash, requestHash) {
			return Child{}, false, ErrIdempotency
		}
		var c Child
		if json.Unmarshal(body, &c) != nil {
			return Child{}, false, errors.New("invalid stored idempotency response")
		}
		return c, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Child{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO idempotency_records(family_id,session_id,route_family,idempotency_key,request_hash,expires_at) VALUES($1,$2,'children.create',$3,$4,now()+interval '24 hours')`, familyID, sessionID, key, requestHash)
	if err != nil {
		return Child{}, false, err
	}
	var pin any
	if pinHash != "" {
		pin = pinHash
	}
	c, err := scanChild(tx.QueryRow(ctx, `INSERT INTO children(family_id,nickname,avatar,color,pin_hash) VALUES($1,$2,$3,$4,$5) RETURNING id,nickname,avatar,color,pin_hash IS NOT NULL,archived_at,created_at,updated_at`, familyID, nickname, avatar, color, pin))
	if err != nil {
		return Child{}, false, mapConstraint(err)
	}
	body, _ = json.Marshal(c)
	_, err = tx.Exec(ctx, `UPDATE idempotency_records SET response_status=201,response_body=$4 WHERE family_id=$1 AND session_id=$2 AND route_family='children.create' AND idempotency_key=$3`, familyID, sessionID, key, body)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status,idempotency_key) VALUES($1,$2,$3,'child.created','child',$4,'active',$5)`, familyID, userID, sessionID, c.ID, key)
	}
	if err != nil {
		return Child{}, false, err
	}
	return c, false, tx.Commit(ctx)
}

type Update struct {
	Nickname, Avatar, Color *string
	PINSet                  bool
	PINHash                 *string
}

func (s *Service) Update(ctx context.Context, sessionID, userID, familyID, childID string, in Update) (Child, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Child{}, err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return Child{}, err
	}
	var pin any
	if in.PINSet && in.PINHash != nil {
		pin = *in.PINHash
	}
	c, err := scanChild(tx.QueryRow(ctx, `UPDATE children SET nickname=coalesce($3,nickname),avatar=coalesce($4,avatar),color=coalesce($5,color),pin_hash=CASE WHEN $6 THEN $7 ELSE pin_hash END,updated_at=now() WHERE id=$1 AND family_id=$2 AND archived_at IS NULL RETURNING id,nickname,avatar,color,pin_hash IS NOT NULL,archived_at,created_at,updated_at`, childID, familyID, in.Nickname, in.Avatar, in.Color, in.PINSet, pin))
	if errors.Is(err, pgx.ErrNoRows) {
		return Child{}, ErrNotFound
	}
	if err != nil {
		return Child{}, mapConstraint(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status) VALUES($1,$2,$3,'child.updated','child',$4,'active','active')`, familyID, userID, sessionID, childID)
	if err != nil {
		return Child{}, err
	}
	return c, tx.Commit(ctx)
}

func (s *Service) Archive(ctx context.Context, sessionID, userID, familyID, childID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = requireParent(ctx, tx, sessionID, userID, familyID); err != nil {
		return err
	}
	var archivedAt *time.Time
	if err = tx.QueryRow(ctx, `SELECT archived_at FROM children WHERE id=$1 AND family_id=$2 FOR UPDATE`, childID, familyID).Scan(&archivedAt); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if archivedAt != nil {
		return tx.Commit(ctx)
	}
	tag, err := tx.Exec(ctx, `UPDATE children SET archived_at=now(),updated_at=now() WHERE id=$1 AND family_id=$2 AND archived_at IS NULL`, childID, familyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("child archive lost locked row")
	}
	_, err = tx.Exec(ctx, `UPDATE sessions SET mode='shared',active_child_id=NULL,parent_unlocked_at=NULL WHERE family_id=$1 AND active_child_id=$2 AND revoked_at IS NULL`, familyID, childID)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status) VALUES($1,$2,$3,'child.archived','child',$4,'active','archived')`, familyID, userID, sessionID, childID)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) Enter(ctx context.Context, sessionID, userID, familyID, childID, pin string) (Child, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Child{}, err
	}
	defer tx.Rollback(ctx)
	var hash *string
	c, err := scanChildWithPIN(tx.QueryRow(ctx, `SELECT id,nickname,avatar,color,pin_hash IS NOT NULL,archived_at,created_at,updated_at,pin_hash FROM children WHERE id=$1 AND family_id=$2 AND archived_at IS NULL FOR UPDATE`, childID, familyID), &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Child{}, ErrProfileUnavailable
	}
	if err != nil {
		return Child{}, err
	}
	if hash != nil && pin == "" {
		return Child{}, ErrProfileUnavailable
	}
	if hash != nil && !auth.VerifyPassword(*hash, pin) {
		return Child{}, ErrProfileUnavailable
	}
	tag, err := tx.Exec(ctx, `UPDATE sessions SET mode='child',active_child_id=$2,parent_unlocked_at=NULL,last_activity_at=now() WHERE id=$1 AND user_id=$4 AND family_id=$3 AND revoked_at IS NULL AND expires_at>now()`, sessionID, childID, familyID, userID)
	if err != nil || tag.RowsAffected() != 1 {
		return Child{}, auth.ErrUnauthenticated
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status) VALUES($1,$2,$3,'child.session_entered','child',$4,NULL,'child')`, familyID, userID, sessionID, childID)
	if err != nil {
		return Child{}, err
	}
	return c, tx.Commit(ctx)
}

func scanChildWithPIN(row pgx.Row, hash **string) (Child, error) {
	var c Child
	var archived *time.Time
	err := row.Scan(&c.ID, &c.Nickname, &c.Avatar, &c.Color, &c.PINEnabled, &archived, &c.CreatedAt, &c.UpdatedAt, hash)
	c.Active = archived == nil
	return c, err
}

func (s *Service) Leave(ctx context.Context, sessionID, userID, familyID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var previousChild *string
	err = tx.QueryRow(ctx, `SELECT active_child_id::text FROM sessions WHERE id=$1 AND user_id=$2 AND family_id=$3 AND revoked_at IS NULL AND expires_at>now() FOR UPDATE`, sessionID, userID, familyID).Scan(&previousChild)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrUnauthenticated
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE sessions SET mode='shared',active_child_id=NULL,parent_unlocked_at=NULL WHERE id=$1`, sessionID)
	if err != nil {
		return err
	}
	if previousChild != nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,before_status,after_status) VALUES($1,$2,$3,'child.session_left','child',$4,'child','shared')`, familyID, userID, sessionID, *previousChild)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func requireParent(ctx context.Context, tx pgx.Tx, sessionID, userID, familyID string) error {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM sessions s
		JOIN family_memberships m ON m.family_id=s.family_id AND m.user_id=s.user_id
		JOIN families f ON f.id=s.family_id
		JOIN users u ON u.id=s.user_id
		WHERE s.id=$1 AND s.user_id=$2 AND s.family_id=$3 AND s.mode='parent'
		  AND s.revoked_at IS NULL AND s.expires_at>now() AND u.active AND m.role='owner'
		  AND now() < s.last_activity_at + make_interval(mins => f.parent_idle_minutes)
	)`, sessionID, userID, familyID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrParentAuthority
	}
	return nil
}

func mapConstraint(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "children_active_nickname_unique" {
		return ErrNicknameExists
	}
	return err
}
func equalHash(a, b []byte) bool        { return len(a) == len(b) && sha256.Sum256(a) == sha256.Sum256(b) }
func NormalizeNickname(v string) string { return strings.TrimSpace(v) }
