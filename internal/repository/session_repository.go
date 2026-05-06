package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/model"
)

// SessionRepository manages user conversation state.
type SessionRepository struct {
	db  *sqlx.DB
	ttl time.Duration
}

func NewSessionRepository(db *sqlx.DB, ttl time.Duration) *SessionRepository {
	return &SessionRepository{db: db, ttl: ttl}
}

// GetOrCreate returns the existing session for a phone number, or creates a fresh one.
func (r *SessionRepository) GetOrCreate(ctx context.Context, phone string) (*model.Session, error) {
    const selectSQL = `
        SELECT id, phone_number, state, last_intent, last_product_id, last_product_name,
               pending_order_id, context, updated_at, expires_at
        FROM sessions
        WHERE phone_number = $1 AND expires_at > NOW()`

    var sess model.Session
    err := r.db.GetContext(ctx, &sess, selectSQL, phone)
    if err == nil {
        sess.LoadContext() // ← isi SearchResults dari Context
        return &sess, nil
    }
    if !errors.Is(err, sql.ErrNoRows) {
        return nil, err
    }

    sess = model.Session{
        PhoneNumber: phone,
        State:       model.StateIdle,
        LastIntent:  string(model.IntentGreeting),
        ExpiresAt:   time.Now().Add(r.ttl),
    }
    return &sess, r.Save(ctx, &sess)
}

func (r *SessionRepository) Save(ctx context.Context, sess *model.Session) error {
    sess.SaveContext() // ← encode SearchResults ke Context sebelum save

    const upsertSQL = `
        INSERT INTO sessions
          (phone_number, state, last_intent, last_product_id, last_product_name,
           pending_order_id, context, updated_at, expires_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)
        ON CONFLICT (phone_number) DO UPDATE SET
          state             = EXCLUDED.state,
          last_intent       = EXCLUDED.last_intent,
          last_product_id   = EXCLUDED.last_product_id,
          last_product_name = EXCLUDED.last_product_name,
          pending_order_id  = EXCLUDED.pending_order_id,
          context           = EXCLUDED.context,
          updated_at        = NOW(),
          expires_at        = EXCLUDED.expires_at`

    _, err := r.db.ExecContext(ctx, upsertSQL,
        sess.PhoneNumber, sess.State, sess.LastIntent,
        sess.LastProductID, sess.LastProductName,
        sess.PendingOrderID, sess.Context,
        sess.ExpiresAt,
    )
    return err
}

// Reset clears the session back to idle state (called after order completion or cancel).
func (r *SessionRepository) Reset(ctx context.Context, phone string) error {
	const sql = `
		UPDATE sessions
		SET state = 'idle', last_intent = '', last_product_id = NULL,
		    last_product_name = '', pending_order_id = NULL, context = '',
		    expires_at = $1, updated_at = NOW()
		WHERE phone_number = $2
	`
	_, err := r.db.ExecContext(ctx, sql, time.Now().Add(r.ttl), phone)
	return err
}

// Extend refreshes the TTL of the session without changing state.
func (r *SessionRepository) Extend(ctx context.Context, phone string, ttl time.Duration) error {
	const sql = `UPDATE sessions SET expires_at = $1, updated_at = NOW() WHERE phone_number = $2`
	_, err := r.db.ExecContext(ctx, sql, time.Now().Add(ttl), phone)
	return err
}