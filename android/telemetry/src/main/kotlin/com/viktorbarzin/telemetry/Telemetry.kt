package com.viktorbarzin.telemetry

import android.content.Context
import android.util.Log
import com.segment.analytics.kotlin.core.Analytics
import io.sentry.android.core.SentryAndroid
import kotlinx.serialization.json.JsonObject
import com.segment.analytics.kotlin.android.Analytics as AndroidAnalytics

/**
 * The app's telemetry: Product events through analytics-kotlin to the Event
 * pipeline, crashes through Sentry to GlitchTip.
 *
 * Call [init] once from `Application.onCreate`. Outside production every call
 * is validated and nothing is sent.
 */
public object Telemetry {
    private const val TAG = "Telemetry"

    @Volatile
    private var events: Events = Events(Config("unconfigured.invalid", "unset", "0"), null, ::warn)

    private fun warn(message: String) {
        Log.w(TAG, message)
    }

    /** True once [init] ran with a production [Config]. */
    public val enabled: Boolean get() = events.enabled

    public fun init(context: Context, config: Config) {
        val app = context.applicationContext
        var sink: EventSink? = null
        if (config.isProduction) {
            SegmentSetup.disableSegmentTelemetry()
            val analytics = AndroidAnalytics(config.writeKey, app) {
                SegmentSetup.apply(this, config)
            }
            sink = SegmentSink(analytics)
            if (!config.dsn.isNullOrBlank()) {
                SentryAndroid.init(app) { options -> SentrySetup.apply(options, config) }
            }
        }
        events = Events(config, sink, ::warn)
    }

    /** A Product event named `noun.past_verb`, e.g. `recording.started`. False when dropped as invalid. */
    public fun track(event: String, properties: JsonObject = emptyProperties): Boolean =
        events.track(event, properties)

    /** A screen view, e.g. `Live`. */
    public fun screen(name: String, properties: JsonObject = emptyProperties): Boolean =
        events.screen(name, properties)

    /** Tie this device's anonymousId to the Person's Authentik `sub`. */
    public fun identify(userId: String, traits: JsonObject = emptyProperties): Boolean =
        events.identify(userId, traits)

    /** Forget the Person and start a new anonymousId, on sign-out. */
    public fun reset() {
        events.reset()
    }

    /** Upload queued events now. */
    public fun flush() {
        events.flush()
    }
}

private class SegmentSink(private val analytics: Analytics) : EventSink {
    override fun track(event: String, properties: JsonObject) = analytics.track(event, properties)

    override fun screen(name: String, properties: JsonObject) = analytics.screen(name, properties)

    override fun identify(userId: String, traits: JsonObject) = analytics.identify(userId, traits)

    override fun reset() = analytics.reset()

    override fun flush() = analytics.flush()
}
