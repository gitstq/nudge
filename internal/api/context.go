package api

import "context"

type ctxKey int

const principalKey ctxKey = iota

func withPrincipal(ctx context.Context, p *principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

func principalFrom(ctx context.Context) *principal {
	if p, ok := ctx.Value(principalKey).(*principal); ok {
		return p
	}
	return nil
}
