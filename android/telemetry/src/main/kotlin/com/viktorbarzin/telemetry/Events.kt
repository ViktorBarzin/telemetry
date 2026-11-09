package com.viktorbarzin.telemetry

import kotlinx.serialization.json.JsonObject

/** Where valid events go. The SDK in the app, a fake in tests. */
internal interface EventSink {
    fun track(event: String, properties: JsonObject)
    fun screen(name: String, properties: JsonObject)
    fun identify(userId: String, traits: JsonObject)
    fun reset()
    fun flush()
}

internal val emptyProperties: JsonObject = JsonObject(emptyMap())

/** Validates every call; forwards to [sink] only in production. */
internal class Events(config: Config, sink: EventSink?, private val warn: (String) -> Unit) {
    private val sink: EventSink? = sink.takeIf { config.isProduction }

    val enabled: Boolean get() = sink != null

    private fun dropped(kind: String, problem: String): Boolean {
        warn("$kind dropped: $problem")
        return false
    }

    fun track(event: String, properties: JsonObject = emptyProperties): Boolean {
        val problem = Validation.eventNameProblem(event) ?: Validation.propertiesProblem(properties)
        if (problem != null) return dropped("event", problem)
        sink?.track(event, properties)
        return true
    }

    fun screen(name: String, properties: JsonObject = emptyProperties): Boolean {
        val problem = (if (name.isBlank()) "screen without a name" else null)
            ?: Validation.propertiesProblem(properties)
        if (problem != null) return dropped("screen", problem)
        sink?.screen(name, properties)
        return true
    }

    fun identify(userId: String, traits: JsonObject = emptyProperties): Boolean {
        val problem = (if (userId.isBlank()) "identify without a user id" else null)
            ?: Validation.propertiesProblem(traits)
        if (problem != null) return dropped("identify", problem)
        sink?.identify(userId, traits)
        return true
    }

    fun reset() {
        sink?.reset()
    }

    fun flush() {
        sink?.flush()
    }
}
