package com.viktorbarzin.telemetry

import kotlinx.serialization.json.JsonObject

/** The Product event rules the Event pipeline also enforces. */
public object Validation {
    public const val MAX_PROPERTIES_LENGTH: Int = 2048

    private val eventName = Regex("[a-z][a-z0-9_]*\\.[a-z][a-z0-9_]*")
    private val propertyKey = Regex("[a-z][a-z0-9_]*")

    /** Why [name] is not `noun.past_verb`, or null. */
    public fun eventNameProblem(name: String): String? =
        if (eventName.matches(name)) null else "event name \"$name\" is not noun.past_verb"

    /** Why [properties] would be rejected (key not snake_case, or over 2,048 bytes as JSON), or null. */
    public fun propertiesProblem(properties: JsonObject): String? {
        properties.keys.firstOrNull { !propertyKey.matches(it) }?.let {
            return "property key \"$it\" is not snake_case"
        }
        val length = properties.toString().toByteArray(Charsets.UTF_8).size
        return if (length > MAX_PROPERTIES_LENGTH) {
            "properties are $length bytes as JSON, over $MAX_PROPERTIES_LENGTH"
        } else {
            null
        }
    }
}
