from __future__ import annotations

import json
import re

import pytest
from hypothesis import given
from hypothesis import strategies as st

from viktorbarzin_telemetry import (
    MAX_PROPERTIES_LENGTH,
    JsonValue,
    event_name_problem,
    properties_problem,
)

EVENT_RE = re.compile(r"^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$")
KEY_RE = re.compile(r"^[a-z][a-z0-9_]*$")

segment = st.from_regex(r"\A[a-z][a-z0-9_]{0,15}\Z")
snake_key = st.from_regex(r"\A[a-z][a-z0-9_]{0,15}\Z")


@given(noun=segment, verb=segment)
def test_well_formed_names_pass(noun: str, verb: str) -> None:
    assert event_name_problem(f"{noun}.{verb}") is None


@given(name=st.text(max_size=40))
def test_names_agree_with_the_pipeline_pattern(name: str) -> None:
    assert (event_name_problem(name) is None) == bool(EVENT_RE.fullmatch(name))


@pytest.mark.parametrize(
    "name",
    [
        "",
        "recording",
        "Recording.started",
        "recording.Started",
        "recording.started.now",
        "1recording.started",
        "recording-x.started",
        "recording.started\n",
        " recording.started",
        "recording.",
        ".started",
    ],
)
def test_malformed_names_fail(name: str) -> None:
    assert event_name_problem(name) is not None


@given(keys=st.lists(snake_key, max_size=5, unique=True))
def test_snake_case_keys_pass(keys: list[str]) -> None:
    props: dict[str, JsonValue] = {k: 1 for k in keys}
    assert properties_problem(props) is None


@given(key=st.text(min_size=0, max_size=20).filter(lambda k: not KEY_RE.fullmatch(k)))
def test_other_keys_fail(key: str) -> None:
    problem = properties_problem({key: 1})
    assert problem is not None
    assert "snake_case" in problem


@pytest.mark.parametrize("key", ["camelCase", "kebab-key", "with space", "_leading", "9lives", ""])
def test_named_bad_keys_fail(key: str) -> None:
    assert properties_problem({key: "x"}) is not None


def _length(props: dict[str, JsonValue]) -> int:
    return len(json.dumps(props, separators=(",", ":"), ensure_ascii=False).encode())


@given(size=st.integers(min_value=0, max_value=4000))
def test_length_limit_is_2048_bytes_of_compact_json(size: int) -> None:
    props: dict[str, JsonValue] = {"text": "a" * size}
    problem = properties_problem(props)
    assert (problem is None) == (_length(props) <= MAX_PROPERTIES_LENGTH)


def test_limit_counts_utf8_bytes() -> None:
    # 1,025 two-byte characters fit as characters but not as bytes.
    props: dict[str, JsonValue] = {"t": "é" * 1025}
    assert _length(props) > MAX_PROPERTIES_LENGTH
    assert properties_problem(props) is not None


@pytest.mark.parametrize("value", [float("nan"), float("inf")])
def test_values_json_cannot_carry_fail(value: float) -> None:
    assert properties_problem({"x": value}) is not None


def test_nested_values_are_allowed() -> None:
    assert properties_problem({"items": [1, "a", {"nested": True}], "n": None}) is None
