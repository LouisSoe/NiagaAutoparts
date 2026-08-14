package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/louissoe/niaga-autoparts/internal/utils"
)

// setAuditActor reads the authenticated actor name from ctx and attempts to set
// the PostgreSQL session variable app.current_user inside the given transaction
// using SET LOCAL, scoping the value to the current transaction only.
//
// IMPORTANT: The call is wrapped in a SAVEPOINT so that if SET LOCAL fails for
// any reason (e.g. custom GUC not configured in postgresql.conf for older PG
// versions, or any other transient error), the transaction is NOT aborted and
// the surrounding DML (INSERT/UPDATE/DELETE) can still succeed.
// In that case audit logs will fall back to recording 'system'.
//
// NOTE: PostgreSQL's SET command does NOT support parameterized placeholders ($1),
// so we use fmt.Sprintf with manual single-quote escaping.
func setAuditActor(ctx context.Context, tx *sqlx.Tx) {
	actor := utils.ActorFromContext(ctx)
	if actor == "" {
		return
	}

	// Create a savepoint before SET LOCAL.
	// If SAVEPOINT itself fails (shouldn't happen in a valid tx), bail out silently.
	if _, err := tx.ExecContext(ctx, "SAVEPOINT _audit_sp"); err != nil {
		return
	}

	// Escape single quotes to prevent SQL injection (e.g. actor = "O'Brien").
	safeActor := strings.ReplaceAll(actor, "'", "''")
	// IMPORTANT: "app.current_user" must be quoted because current_user is a
	// reserved PostgreSQL keyword. Without quotes, PostgreSQL throws 42601 (syntax error).
	_, setErr := tx.ExecContext(ctx, fmt.Sprintf(`SET LOCAL "app.current_user" = '%s'`, safeActor))
	if setErr != nil {
		// SET LOCAL failed — roll back to savepoint so the tx stays healthy.
		_, _ = tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT _audit_sp")
		return
	}

	// Success — release the savepoint (no longer needed).
	_, _ = tx.ExecContext(ctx, "RELEASE SAVEPOINT _audit_sp")
}
