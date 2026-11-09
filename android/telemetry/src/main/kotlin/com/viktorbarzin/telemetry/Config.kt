package com.viktorbarzin.telemetry

/**
 * One app's telemetry settings.
 *
 * @property appHost the app's public host, e.g. `tv.viktorbarzin.me`. The Event
 *   pipeline and GlitchTip answer on its `/_t/` path.
 * @property writeKey the app's Segment write key.
 * @property release the app version, sent as the Sentry release and `context.app.version`.
 * @property dsn the app's GlitchTip DSN, `https://<key>@<app host>/_t/x/<project id>`.
 * @property environment events and errors leave the device only when this is
 *   `production`. Release builds pass [PRODUCTION]; the default sends nothing.
 */
public data class Config(
    val appHost: String,
    val writeKey: String,
    val release: String,
    val dsn: String? = null,
    val environment: String = DEV,
) {
    /** [appHost] without a scheme or trailing slash. */
    val host: String = appHost.trim().removePrefix("https://").removePrefix("http://").trimEnd('/')

    /** Where analytics-kotlin uploads (`https://<apiHost>/b`). */
    val apiHost: String get() = "$host/_t/e/v1"

    /**
     * Where analytics-kotlin fetches its settings. The pipeline answers with a
     * non-200, so the SDK uses [SegmentSetup.defaultSettings].
     */
    val cdnHost: String get() = apiHost

    val isProduction: Boolean get() = environment == PRODUCTION

    init {
        require(host.isNotEmpty() && '/' !in host) { "appHost must be a bare host such as tv.viktorbarzin.me" }
        require(writeKey.isNotBlank()) { "writeKey must not be blank" }
    }

    public companion object {
        public const val PRODUCTION: String = "production"
        public const val DEV: String = "dev"
    }
}
