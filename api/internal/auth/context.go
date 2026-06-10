package auth

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey string

const userIDKey ctxKey = "user_id"

func withUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok
}
