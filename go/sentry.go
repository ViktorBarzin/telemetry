package telemetry

import "github.com/getsentry/sentry-go"

// SentryOptions are the sentry-go options for cfg: DSN, release and environment.
func SentryOptions(cfg Config) sentry.ClientOptions {
	return sentry.ClientOptions{
		Dsn:         cfg.DSN,
		Release:     cfg.Release,
		Environment: cfg.environment(),
	}
}

// InitSentry starts sentry-go against GlitchTip when cfg is production and
// has a DSN, and reports whether it did. Call sentry.Flush before exit.
func InitSentry(cfg Config) (bool, error) {
	if cfg.DSN == "" || !cfg.IsProduction() {
		return false, nil
	}
	if err := sentry.Init(SentryOptions(cfg)); err != nil {
		return false, err
	}
	return true, nil
}
