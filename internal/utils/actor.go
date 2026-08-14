package utils

import "context"

// actorKeyType is an unexported type for the audit actor context key,
// preventing collisions with other packages that store values in the same context.
type actorKeyType struct{}

// actorKey is the singleton context key for the audit actor name.
var actorKey = actorKeyType{}

// WithActor returns a new context that carries the given actor name.
// This should be called by AuthMiddleware after validating the JWT, so that
// downstream service and repository layers can propagate the actor into
// PostgreSQL audit triggers via SET LOCAL app.current_user.
func WithActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, actorKey, actor)
}

// ActorFromContext retrieves the actor name stored by WithActor.
// Returns an empty string if no actor has been set (e.g. background workers,
// unauthenticated requests), which causes the audit trigger to fall back to 'system'.
func ActorFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(actorKey).(string); ok {
		return v
	}
	return ""
}
