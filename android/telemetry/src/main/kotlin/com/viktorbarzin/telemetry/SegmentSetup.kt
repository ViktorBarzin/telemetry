package com.viktorbarzin.telemetry

import com.segment.analytics.kotlin.core.Configuration
import com.segment.analytics.kotlin.core.Settings
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import com.segment.analytics.kotlin.core.Telemetry as SegmentTelemetry

/** analytics-kotlin configuration for our Event pipeline. Uses only the SDK's JVM core. */
public object SegmentSetup {
    /**
     * The settings the SDK falls back to when its settings request fails, which
     * it always does against our pipeline. The Segment.io entry must carry
     * apiKey and apiHost: without apiHost the SDK's own default
     * (`api.segment.io/v1`) replaces [Configuration.apiHost].
     */
    public fun defaultSettings(config: Config): Settings = Settings(
        integrations = buildJsonObject {
            put(
                "Segment.io",
                buildJsonObject {
                    put("apiKey", config.writeKey)
                    put("apiHost", config.apiHost)
                },
            )
        },
    )

    /** Point [configuration] at the pipeline on [Config.host]. */
    public fun apply(configuration: Configuration, config: Config) {
        configuration.apiHost = config.apiHost
        configuration.cdnHost = config.cdnHost
        configuration.defaultSettings = defaultSettings(config)
        configuration.collectDeviceId = false
        configuration.trackApplicationLifecycleEvents = false
        configuration.trackDeepLinks = false
    }

    /** Stop the SDK reporting its own usage metrics to Segment. */
    public fun disableSegmentTelemetry() {
        SegmentTelemetry.enable = false
    }
}
