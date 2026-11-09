package com.viktorbarzin.telemetry

import com.segment.analytics.kotlin.core.Configuration
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Test
import com.segment.analytics.kotlin.core.Telemetry as SegmentTelemetry

class SegmentSetupTest {
    private val config = Config("tv.viktorbarzin.me", "wk-123", "1.2.3", environment = "production")

    @Test
    fun defaultSettingsPointTheSegmentDestinationAtOurPipeline() {
        // The pipeline answers the settings request with a non-200, so the SDK
        // falls back to these. Without apiHost here it would reset the upload
        // host to api.segment.io.
        val segment = SegmentSetup.defaultSettings(config).integrations["Segment.io"]?.jsonObject
        assertNotNull(segment)
        assertEquals("wk-123", segment!!["apiKey"]?.jsonPrimitive?.content)
        assertEquals("tv.viktorbarzin.me/_t/e/v1", segment["apiHost"]?.jsonPrimitive?.content)
    }

    @Test
    fun configurationUsesOurHosts() {
        val configuration = Configuration(writeKey = config.writeKey)
        SegmentSetup.apply(configuration, config)
        assertEquals("tv.viktorbarzin.me/_t/e/v1", configuration.apiHost)
        assertEquals("tv.viktorbarzin.me/_t/e/v1", configuration.cdnHost)
        assertEquals(SegmentSetup.defaultSettings(config), configuration.defaultSettings)
        assertFalse(configuration.collectDeviceId)
    }

    @Test
    fun segmentsOwnTelemetryIsOff() {
        SegmentTelemetry.enable = true
        SegmentSetup.disableSegmentTelemetry()
        assertFalse(SegmentTelemetry.enable)
    }
}
