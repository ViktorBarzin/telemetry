package com.viktorbarzin.telemetry

import io.sentry.SentryOptions
import org.junit.Assert.assertEquals
import org.junit.Test

class SentrySetupTest {
    @Test
    fun optionsCarryDsnReleaseAndEnvironment() {
        val dsn = "https://key@tv.viktorbarzin.me/_t/x/1"
        val config = Config("tv.viktorbarzin.me", "wk", "1.2.3", dsn = dsn, environment = "production")
        val options = SentryOptions()
        SentrySetup.apply(options, config)
        assertEquals(dsn, options.dsn)
        assertEquals("1.2.3", options.release)
        assertEquals("production", options.environment)
    }
}
