package kitapitest

import (
	"context"
	"net/http"
)

type identityKey struct{}

func withIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

func identity(r *http.Request) Identity {
	id, _ := r.Context().Value(identityKey{}).(Identity)
	return id
}
