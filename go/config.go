package telemetry

import "os"

const (
	// Production is the only environment in which events and errors are sent.
	Production = "production"
	// EnvironmentVariable is read when Config.Environment is empty.
	EnvironmentVariable = "TELEMETRY_ENVIRONMENT"
	// HostVariable is read when Config.Endpoint is empty.
	HostVariable = "TELEMETRY_HOST"
)

// PublicEndpoint is the Event pipeline on an app's public host, for code
// running outside the cluster.
func PublicEndpoint(appHost string) string {
	return "https://" + appHost + "/_t/e"
}

// ResolveEnvironment returns explicit, else $TELEMETRY_ENVIRONMENT, else "dev".
func ResolveEnvironment(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv(EnvironmentVariable); env != "" {
		return env
	}
	return "dev"
}

// Config is one app's telemetry settings.
type Config struct {
	// App is the app's name, the same one its Write key is registered under.
	App string
	// WriteKey is the app's Segment write key from Vault secret/rybbit.
	WriteKey string
	// Release is the app version, sent as service.version and the Sentry release.
	Release string
	// Environment is "production" or anything else; empty reads $TELEMETRY_ENVIRONMENT.
	Environment string
	// Endpoint is the Event pipeline's base URL; analytics-go appends
	// /v1/batch. Empty reads $TELEMETRY_HOST. Production needs one: inside
	// our cluster it is the pipeline's Service, elsewhere PublicEndpoint.
	Endpoint string
	// DSN is the app's GlitchTip DSN, https://<key>@<app host>/_t/x/<project id>.
	DSN string
	// DefaultAnonymousID is the anonymousId of events with neither a Person
	// nor a visitor id, e.g. from a worker. Empty means "server:<App>".
	DefaultAnonymousID string
}

func (c Config) fallbackAnonymousID() string {
	if c.DefaultAnonymousID != "" {
		return c.DefaultAnonymousID
	}
	return "server:" + c.App
}

func (c Config) environment() string { return ResolveEnvironment(c.Environment) }

func (c Config) endpoint() string {
	if c.Endpoint != "" {
		return c.Endpoint
	}
	return os.Getenv(HostVariable)
}

// IsProduction reports whether this config sends anything.
func (c Config) IsProduction() bool { return c.environment() == Production }
