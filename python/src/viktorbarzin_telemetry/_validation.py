"""The Product event rules the Event pipeline also enforces.

The pipeline rejects an event that breaks them, so the library checks first,
drops the event and logs why, instead of sending something that will be thrown
away.
"""

from __future__ import annotations

import json
import re
from collections.abc import Mapping, Sequence
from typing import TypeAlias

JsonValue: TypeAlias = (
    "str | int | float | bool | Sequence[JsonValue] | Mapping[str, JsonValue] | None"
)
Properties: TypeAlias = Mapping[str, JsonValue]

EVENT_NAME_PATTERN = re.compile(r"[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*")
PROPERTY_KEY_PATTERN = re.compile(r"[a-z][a-z0-9_]*")
MAX_PROPERTIES_LENGTH = 2048


def event_name_problem(name: str) -> str | None:
    """Why ``name`` is not a valid ``noun.past_verb`` event name, or None."""
    if EVENT_NAME_PATTERN.fullmatch(name):
        return None
    return f"event name {name!r} is not noun.past_verb"


def properties_length(properties: Properties) -> int:
    """UTF-8 bytes of the compact JSON encoding, which bounds the pipeline's count."""
    encoded = json.dumps(properties, separators=(",", ":"), ensure_ascii=False, allow_nan=False)
    return len(encoded.encode("utf-8"))


def properties_problem(properties: Properties) -> str | None:
    """Why ``properties`` would be rejected, or None when they are fine."""
    for key in properties:
        if not PROPERTY_KEY_PATTERN.fullmatch(key):
            return f"property key {key!r} is not snake_case"
    try:
        length = properties_length(properties)
    except (TypeError, ValueError) as err:
        return f"properties are not JSON: {err}"
    if length > MAX_PROPERTIES_LENGTH:
        return f"properties are {length} bytes as JSON, over {MAX_PROPERTIES_LENGTH}"
    return None
