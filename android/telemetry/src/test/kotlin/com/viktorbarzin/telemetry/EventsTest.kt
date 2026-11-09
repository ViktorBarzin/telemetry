package com.viktorbarzin.telemetry

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.junit.runners.Parameterized
import org.junit.runners.Parameterized.Parameters

private class FakeSink : EventSink {
    val calls = mutableListOf<String>()

    override fun track(event: String, properties: JsonObject) {
        calls += "track $event $properties"
    }

    override fun screen(name: String, properties: JsonObject) {
        calls += "screen $name $properties"
    }

    override fun identify(userId: String, traits: JsonObject) {
        calls += "identify $userId $traits"
    }

    override fun reset() {
        calls += "reset"
    }

    override fun flush() {
        calls += "flush"
    }
}

private fun events(environment: String = "production"): Triple<Events, FakeSink, MutableList<String>> {
    val sink = FakeSink()
    val warnings = mutableListOf<String>()
    val config = Config("tv.viktorbarzin.me", "wk", "1", environment = environment)
    return Triple(Events(config, sink) { warnings += it }, sink, warnings)
}

@RunWith(Parameterized::class)
class InvalidEventTest(private val event: String, private val properties: JsonObject) {
    companion object {
        @JvmStatic
        @Parameters(name = "{0} {1}")
        fun cases(): List<Array<Any>> = listOf(
            arrayOf("Trip Created", JsonObject(emptyMap())),
            arrayOf("trip.created", buildJsonObject { put("tripId", 1) }),
            arrayOf("trip.created", buildJsonObject { put("blob", "x".repeat(3000)) }),
        )
    }

    @Test
    fun isDroppedWithAWarning() {
        val (events, sink, warnings) = events()
        assertFalse(events.track(event, properties))
        assertEquals(emptyList<String>(), sink.calls)
        assertTrue(warnings.single().contains("dropped"))
    }
}

@RunWith(Parameterized::class)
class OutsideProductionTest(private val environment: String) {
    companion object {
        @JvmStatic
        @Parameters(name = "{0}")
        fun cases(): List<Array<Any>> = listOf("dev", "", "staging").map { arrayOf(it) }
    }

    @Test
    fun sendsNothingButStillValidates() {
        val (events, sink, _) = events(environment)
        assertFalse(events.enabled)
        assertTrue(events.track("trip.created"))
        assertTrue(events.screen("Home"))
        assertTrue(events.identify("sub-1"))
        events.reset()
        events.flush()
        assertEquals(emptyList<String>(), sink.calls)
        assertFalse(events.track("BadName"))
    }
}

class EventsTest {
    @Test
    fun validCallsReachTheSink() {
        val (events, sink, warnings) = events()
        assertTrue(events.enabled)
        assertTrue(events.track("recording.started", buildJsonObject { put("channel_id", 4) }))
        assertTrue(events.screen("Live", buildJsonObject { put("channel_id", 4) }))
        assertTrue(events.identify("sub-1", buildJsonObject { put("plan", "free") }))
        events.reset()
        events.flush()
        assertEquals(
            listOf(
                """track recording.started {"channel_id":4}""",
                """screen Live {"channel_id":4}""",
                """identify sub-1 {"plan":"free"}""",
                "reset",
                "flush",
            ),
            sink.calls,
        )
        assertEquals(emptyList<String>(), warnings)
    }

    @Test
    fun identifyNeedsAUserId() {
        val (events, sink, warnings) = events()
        assertFalse(events.identify(" "))
        assertEquals(emptyList<String>(), sink.calls)
        assertEquals(1, warnings.size)
    }

    @Test
    fun screenNeedsANameAndValidProperties() {
        val (events, sink, _) = events()
        assertFalse(events.screen(""))
        assertFalse(events.screen("Home", buildJsonObject { put("Bad", 1) }))
        assertEquals(emptyList<String>(), sink.calls)
    }
}
