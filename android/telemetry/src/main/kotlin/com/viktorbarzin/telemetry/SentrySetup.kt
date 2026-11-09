package com.viktorbarzin.telemetry

import io.sentry.SentryOptions

/** Sentry options for GlitchTip. */
public object SentrySetup {
    public fun apply(options: SentryOptions, config: Config) {
        options.dsn = config.dsn
        options.release = config.release
        options.environment = config.environment
    }
}
