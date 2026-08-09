package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const AbsoluteLifetime = 30 * 24 * time.Hour

var ErrUnauthenticated = errors.New("unauthenticated")

type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool, now: time.Now} }

type Session struct {
	ID, UserID, FamilyID             string
	Mode                             string
	ActiveChildID                    string
	CSRFToken                        string
	IdleExpiresAt, AbsoluteExpiresAt time.Time
}

func (s *Service) Register(ctx context.Context, email, password, familyName, timezone string, weekStart int16) (Session, string, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return Session{}, "", err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Session{}, "", err
	}
	defer tx.Rollback(ctx)
	var userID, familyID string
	if err = tx.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES(lower($1),$2) RETURNING id`, email, hash).Scan(&userID); err != nil {
		return Session{}, "", err
	}
	if err = tx.QueryRow(ctx, `INSERT INTO families(name,timezone,week_starts_on) VALUES($1,$2,$3) RETURNING id`, familyName, timezone, weekStart).Scan(&familyID); err != nil {
		return Session{}, "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO family_memberships(family_id,user_id,role) VALUES($1,$2,'owner')`, familyID, userID); err != nil {
		return Session{}, "", err
	}
	session, token, err := s.createSession(ctx, tx, userID, familyID)
	if err != nil {
		return Session{}, "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status) VALUES($1,$2,$3,'parent.registered','user',$2,'active')`, familyID, userID, session.ID); err != nil {
		return Session{}, "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return Session{}, "", err
	}
	return session, token, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (Session, string, error) {
	var userID, familyID, hash string
	err := s.pool.QueryRow(ctx, `SELECT u.id,m.family_id,u.password_hash FROM users u JOIN family_memberships m ON m.user_id=u.id WHERE lower(u.email)=lower($1) AND u.active`, email).Scan(&userID, &familyID, &hash)
	if err != nil || !VerifyPassword(hash, password) {
		return Session{}, "", ErrInvalidCredentials
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Session{}, "", err
	}
	defer tx.Rollback(ctx)
	session, token, err := s.createSession(ctx, tx, userID, familyID)
	if err != nil {
		return Session{}, "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(family_id,actor_user_id,session_id,action,subject_type,subject_id,after_status) VALUES($1,$2,$3,'parent.logged_in','session',$3,'active')`, familyID, userID, session.ID)
	if err != nil {
		return Session{}, "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return Session{}, "", err
	}
	return session, token, nil
}

type dbtx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) createSession(ctx context.Context, q dbtx, userID, familyID string) (Session, string, error) {
	token, tokenHash, err := credential()
	if err != nil {
		return Session{}, "", err
	}
	csrf, csrfHash, err := credential()
	if err != nil {
		return Session{}, "", err
	}
	now := s.now().UTC()
	absolute := now.Add(AbsoluteLifetime)
	var id string
	err = q.QueryRow(ctx, `INSERT INTO sessions(family_id,user_id,token_hash,csrf_token_hash,mode,parent_unlocked_at,last_activity_at,expires_at) VALUES($1,$2,$3,$4,'parent',$5,$5,$6) RETURNING id`, familyID, userID, tokenHash, csrfHash, now, absolute).Scan(&id)
	return Session{ID: id, UserID: userID, FamilyID: familyID, Mode: "parent", CSRFToken: csrf, IdleExpiresAt: now.Add(15 * time.Minute), AbsoluteExpiresAt: absolute}, token, err
}

func credential() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:], nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Session, error) {
	sum := sha256.Sum256([]byte(token))
	var out Session
	var csrfHash []byte
	var last time.Time
	var idle int
	err := s.pool.QueryRow(ctx, `SELECT s.id,s.user_id,s.family_id,s.mode,coalesce(s.active_child_id::text,''),s.csrf_token_hash,s.last_activity_at,s.expires_at,f.parent_idle_minutes FROM sessions s JOIN families f ON f.id=s.family_id JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now() AND u.active`, sum[:]).Scan(&out.ID, &out.UserID, &out.FamilyID, &out.Mode, &out.ActiveChildID, &csrfHash, &last, &out.AbsoluteExpiresAt, &idle)
	if err != nil {
		return Session{}, ErrUnauthenticated
	}
	now := s.now().UTC()
	out.IdleExpiresAt = last.Add(time.Duration(idle) * time.Minute)
	if out.Mode == "parent" && !now.Before(out.IdleExpiresAt) {
		_, _ = s.pool.Exec(ctx, `UPDATE sessions SET mode='shared',parent_unlocked_at=NULL WHERE id=$1`, out.ID)
		out.Mode = "shared"
	}
	if out.Mode == "child" {
		tag, updateErr := s.pool.Exec(ctx, `UPDATE sessions s SET mode='shared',active_child_id=NULL,parent_unlocked_at=NULL
			WHERE s.id=$1 AND NOT EXISTS (SELECT 1 FROM children c WHERE c.id=s.active_child_id AND c.family_id=s.family_id AND c.archived_at IS NULL)`, out.ID)
		if updateErr != nil {
			return Session{}, ErrUnauthenticated
		}
		if tag.RowsAffected() == 1 {
			out.Mode = "shared"
			out.ActiveChildID = ""
		}
	}
	if out.Mode != "parent" {
		out.IdleExpiresAt = time.Time{}
	}
	return out, nil
}

func (s *Service) CheckCSRF(ctx context.Context, sessionID, token string) bool {
	sum := sha256.Sum256([]byte(token))
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT csrf_token_hash=$2 FROM sessions WHERE id=$1 AND revoked_at IS NULL`, sessionID, sum[:]).Scan(&ok)
	return err == nil && ok
}
func (s *Service) Touch(ctx context.Context, sessionID string) {
	_, _ = s.pool.Exec(ctx, `UPDATE sessions SET last_activity_at=now() WHERE id=$1 AND revoked_at IS NULL`, sessionID)
}
func (s *Service) Logout(ctx context.Context, session Session) error {
	_, err := s.pool.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL`, session.ID)
	return err
}
func (s *Service) UnlockParent(ctx context.Context, session Session, password, pin string) (Session, error) {
	var passwordHash string
	var pinHash *string
	if err := s.pool.QueryRow(ctx, `SELECT password_hash,parent_pin_hash FROM users WHERE id=$1 AND active`, session.UserID).Scan(&passwordHash, &pinHash); err != nil {
		return Session{}, ErrInvalidCredentials
	}
	valid := password != "" && VerifyPassword(passwordHash, password)
	if pin != "" && pinHash != nil {
		valid = VerifyPassword(*pinHash, pin)
	}
	if !valid {
		return Session{}, ErrInvalidCredentials
	}
	now := s.now().UTC()
	if _, err := s.pool.Exec(ctx, `UPDATE sessions SET mode='parent',active_child_id=NULL,parent_unlocked_at=$2,last_activity_at=$2 WHERE id=$1 AND revoked_at IS NULL AND expires_at>$2`, session.ID, now); err != nil {
		return Session{}, err
	}
	var idle int
	if err := s.pool.QueryRow(ctx, `SELECT parent_idle_minutes FROM families WHERE id=$1`, session.FamilyID).Scan(&idle); err != nil {
		return Session{}, err
	}
	session.Mode = "parent"
	session.ActiveChildID = ""
	session.IdleExpiresAt = now.Add(time.Duration(idle) * time.Minute)
	return session, nil
}
func (s *Service) LockParent(ctx context.Context, session Session) (Session, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE sessions SET mode='shared',active_child_id=NULL,parent_unlocked_at=NULL WHERE id=$1 AND revoked_at IS NULL`, session.ID)
	if err != nil || tag.RowsAffected() != 1 {
		return Session{}, ErrUnauthenticated
	}
	session.Mode = "shared"
	session.ActiveChildID = ""
	session.IdleExpiresAt = time.Time{}
	return session, nil
}
func (s *Service) RotateCSRF(ctx context.Context, sessionID string) (string, error) {
	raw, hash, err := credential()
	if err != nil {
		return "", err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE sessions SET csrf_token_hash=$2 WHERE id=$1 AND revoked_at IS NULL`, sessionID, hash)
	if err != nil || tag.RowsAffected() != 1 {
		return "", ErrUnauthenticated
	}
	return raw, nil
}

func NormalizeEmail(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
