package config

import "context"

type contextKey struct{}

func IntoContext(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, contextKey{}, cfg)
}

func FromContext(ctx context.Context) *Config {
	if ctx == nil {
		panic("config: nil context")
	}
	cfg, ok := ctx.Value(contextKey{}).(*Config)
	if !ok || cfg == nil {
		panic("config: configuration missing from context")
	}
	return cfg
}
