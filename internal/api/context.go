package api

import (
	"context"
	"net/http"

	"github.com/Shyyw1e/VoiceCAD/internal/core"
)

type userContextKey struct{}

func withUser(ctx context.Context, user core.User) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func userFromRequest(r *http.Request) core.User {
	user, _ := r.Context().Value(userContextKey{}).(core.User)
	return user
}
