package com.viktorbarzin.telemetry

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import org.junit.runners.Parameterized
import org.junit.runners.Parameterized.Parameters

@RunWith(Parameterized::class)
class EventNameTest(private val name: String, private val valid: Boolean) {
    companion object {
        @JvmStatic
        @Parameters(name = "{0} -> {1}")
        fun cases(): List<Array<Any>> = listOf(
            arrayOf("recording.started", true),
            arrayOf("trip.created", true),
            arrayOf("a1_b.c2_d", true),
            arrayOf("", false),
            arrayOf("recording", false),
            arrayOf("Recording.started", false),
            arrayOf("recording.Started", false),
            arrayOf("recording.started.now", false),
            arrayOf("1recording.started", false),
            arrayOf("recording-x.started", false),
            arrayOf("recording.started\n", false),
            arrayOf(" recording.started", false),
            arrayOf("recording.", false),
            arrayOf(".started", false),
        )
    }

    @Test
    fun matchesThePipelineRule() {
        assertEquals(valid, Validation.eventNameProblem(name) == null)
    }
}

@RunWith(Parameterized::class)
class PropertyKeyTest(private val key: String, private val valid: Boolean) {
    companion object {
        @JvmStatic
        @Parameters(name = "{0} -> {1}")
        fun cases(): List<Array<Any>> = listOf(
            arrayOf("channel_id", true),
            arrayOf("x1", true),
            arrayOf("channelId", false),
            arrayOf("a-b", false),
            arrayOf("_a", false),
            arrayOf("9a", false),
            arrayOf("", false),
            arrayOf("with space", false),
        )
    }

    @Test
    fun matchesThePipelineRule() {
        val props = JsonObject(mapOf(key to JsonPrimitive(1)))
        assertEquals(valid, Validation.propertiesProblem(props) == null)
    }
}

@RunWith(Parameterized::class)
class PropertiesLengthTest(private val size: Int) {
    companion object {
        @JvmStatic
        @Parameters(name = "{0} chars")
        fun cases(): List<Array<Any>> = listOf(0, 1, 2000, 2036, 2037, 2038, 4000).map { arrayOf(it) }
    }

    @Test
    fun limitIs2048BytesOfCompactJson() {
        val props = buildJsonObject { put("text", "a".repeat(size)) }
        val bytes = props.toString().toByteArray(Charsets.UTF_8).size
        assertEquals(bytes <= Validation.MAX_PROPERTIES_LENGTH, Validation.propertiesProblem(props) == null)
    }
}

class ValidationTest {
    @Test
    fun countsUtf8BytesNotCharacters() {
        val props = buildJsonObject { put("t", "é".repeat(1025)) }
        assertNotNull(Validation.propertiesProblem(props))
    }

    @Test
    fun nestedValuesAreAllowed() {
        val props = buildJsonObject {
            put("items", buildJsonArray { add(JsonPrimitive(1)); add(buildJsonObject { put("Nested", true) }) })
        }
        assertNull(Validation.propertiesProblem(props))
    }
}
