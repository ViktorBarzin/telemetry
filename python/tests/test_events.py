from __future__ import annotations

import logging
from collections.abc import Mapping
from dataclasses import dataclass, field

import pytest
from hypothesis import given
from hypothesis import strategies as st

import viktorbarzin_telemetry as telemetry
from viktorbarzin_telemetry import (
    Config,
    JsonValue,
    Telemetry,
    log_context,
    resolve_environment,
    resolve_host,
)


@dataclass
class Sent:
    kind: str
    user_id: str | None
    anonymous_id: str | None
    name: str
    payload: Mapping[str, JsonValue]
    context: Mapping[str, JsonValue]


@dataclass
class FakeClient:
    sent: list[Sent] = field(default_factory=list)
    flushes: int = 0

    def track(
        self,
        *,
        user_id: str | None,
        event: str,
        properties: dict[str, JsonValue],
        context: dict[str, JsonValue],
        anonymous_id: str | None,
    ) -> object:
        self.sent.append(Sent("track", user_id, anonymous_id, event, properties, context))
        return None

    def identify(
        self,
        *,
        user_id: str,
        traits: dict[str, JsonValue],
        context: dict[str, JsonValue],
        anonymous_id: str | None,
    ) -> object:
        self.sent.append(Sent("identify", user_id, anonymous_id, "", traits, context))
        return None

    def flush(self) -> None:
        self.flushes += 1


def _config(environment: str = "production") -> Config:
    return Config(app="tv", write_key="wk", release="1.2.3", environment=environment)


def _prod() -> tuple[Telemetry, FakeClient]:
    fake = FakeClient()
    return Telemetry(_config(), client=fake), fake


def test_track_sends_a_valid_event_with_app_context() -> None:
    t, fake = _prod()
    assert t.track("recording.started", {"channel_id": 4}, user_id="sub-1", anonymous_id="anon-1")
    [sent] = fake.sent
    assert (sent.kind, sent.name, sent.user_id, sent.anonymous_id) == (
        "track",
        "recording.started",
        "sub-1",
        "anon-1",
    )
    assert sent.payload == {"channel_id": 4}
    assert sent.context == {"app": {"name": "tv", "version": "1.2.3"}}


@given(st.integers(min_value=2, max_value=20))
def test_anonymous_events_share_one_stable_id(n: int) -> None:
    t, fake = _prod()
    for _ in range(n):
        t.track("trip.created")
    ids = {s.anonymous_id for s in fake.sent}
    assert ids == {"server:tv"}
    assert all(s.user_id is None for s in fake.sent)


def test_a_caller_supplied_default_anonymous_id_is_used() -> None:
    fake = FakeClient()
    config = Config(
        app="tv",
        write_key="wk",
        release="1",
        environment="production",
        default_anonymous_id="worker-7",
    )
    t = Telemetry(config, client=fake)
    t.track("trip.created")
    t.track("trip.created")
    assert [s.anonymous_id for s in fake.sent] == ["worker-7", "worker-7"]


@pytest.mark.parametrize("kind", ["track", "identify"])
def test_a_known_person_without_a_visitor_sends_no_anonymous_id(kind: str) -> None:
    t, fake = _prod()
    if kind == "track":
        t.track("trip.created", user_id="sub-1")
    else:
        t.identify("sub-1")
    [sent] = fake.sent
    assert (sent.user_id, sent.anonymous_id) == ("sub-1", None)


VISITOR = {"X-Visitor-Id": "v-1", "X-Real-Ip": "81.2.69.142", "User-Agent": "Mozilla/5.0 x"}


@pytest.mark.parametrize(
    ("explicit", "log_anon", "user", "expected"),
    [
        (None, None, None, "v-1"),
        (None, None, "sub-1", "v-1"),
        ("anon-x", None, None, "anon-x"),
        (None, "anon-log", None, "anon-log"),
        ("anon-x", "anon-log", "sub-1", "anon-x"),
    ],
)
def test_anonymous_id_precedence_in_a_request(
    explicit: str | None, log_anon: str | None, user: str | None, expected: str
) -> None:
    t, fake = _prod()
    with telemetry.request_context(VISITOR), log_context(anonymous_id=log_anon):
        t.track("trip.created", user_id=user, anonymous_id=explicit)
        t.identify("sub-1", anonymous_id=explicit)
    tracked, identified = fake.sent
    assert tracked.anonymous_id == identified.anonymous_id == expected


@given(
    ip=st.ip_addresses().map(str),
    agent=st.from_regex(r"Mozilla/5\.0 \([A-Za-z0-9 ;]{1,30}\)", fullmatch=True),
)
def test_request_ip_and_agent_go_in_the_event_context(ip: str, agent: str) -> None:
    t, fake = _prod()
    with telemetry.request_context({"X-Real-Ip": ip, "User-Agent": agent}):
        t.track("trip.created", user_id="sub-1")
        t.identify("sub-1")
    for sent in fake.sent:
        assert sent.context == {
            "app": {"name": "tv", "version": "1.2.3"},
            "ip": ip,
            "userAgent": agent,
        }
    t.track("trip.created")
    assert fake.sent[-1].context == {"app": {"name": "tv", "version": "1.2.3"}}


def test_ids_come_from_log_context() -> None:
    t, fake = _prod()
    with log_context(enduser_id="sub-9", anonymous_id="anon-9"):
        t.track("trip.created")
    [sent] = fake.sent
    assert (sent.user_id, sent.anonymous_id) == ("sub-9", "anon-9")


@pytest.mark.parametrize(
    ("event", "props"),
    [
        ("Trip Created", None),
        ("trip.created", {"tripId": 1}),
        ("trip.created", {"blob": "x" * 3000}),
    ],
)
def test_invalid_events_are_dropped_with_a_warning(
    event: str, props: dict[str, JsonValue] | None, caplog: pytest.LogCaptureFixture
) -> None:
    t, fake = _prod()
    with caplog.at_level(logging.WARNING, logger="viktorbarzin_telemetry"):
        assert not t.track(event, props)
    assert fake.sent == []
    assert any("dropped" in r.getMessage() for r in caplog.records)


@given(environment=st.text(max_size=12).filter(lambda e: e != "production"))
def test_nothing_is_sent_outside_production(environment: str) -> None:
    fake = FakeClient()
    t = Telemetry(_config(environment), client=fake)
    assert t.track("trip.created")
    assert t.identify("sub-1")
    t.flush()
    assert fake.sent == []
    assert fake.flushes == 0
    assert not t.enabled


def test_validation_still_runs_outside_production() -> None:
    t = Telemetry(_config("dev"), client=FakeClient())
    assert not t.track("BadName")


def test_identify_sends_traits() -> None:
    t, fake = _prod()
    assert t.identify("sub-1", {"plan": "free"}, anonymous_id="anon-1")
    [sent] = fake.sent
    assert (sent.kind, sent.user_id, sent.anonymous_id) == ("identify", "sub-1", "anon-1")
    assert sent.payload == {"plan": "free"}


@pytest.mark.parametrize(("user_id", "traits"), [("", None), ("sub-1", {"Plan": "x"})])
def test_identify_rejects_bad_input(user_id: str, traits: dict[str, JsonValue] | None) -> None:
    t, fake = _prod()
    assert not t.identify(user_id, traits)
    assert fake.sent == []


def test_flush_reaches_the_client() -> None:
    t, fake = _prod()
    t.flush()
    assert fake.flushes == 1


@pytest.mark.parametrize(
    ("explicit", "env", "expected"),
    [
        ("production", None, "production"),
        (None, "production", "production"),
        (None, None, "dev"),
        ("dev", "production", "dev"),
        (None, "", "dev"),
    ],
)
def test_resolve_environment(
    explicit: str | None, env: str | None, expected: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    if env is None:
        monkeypatch.delenv("TELEMETRY_ENVIRONMENT", raising=False)
    else:
        monkeypatch.setenv("TELEMETRY_ENVIRONMENT", env)
    assert resolve_environment(explicit) == expected


@pytest.mark.parametrize(
    ("explicit", "env", "expected"),
    [
        ("https://tv.viktorbarzin.me/_t/e", None, "https://tv.viktorbarzin.me/_t/e"),
        (None, "https://tv.viktorbarzin.me/_t/e", "https://tv.viktorbarzin.me/_t/e"),
        (
            "https://a.viktorbarzin.me/_t/e",
            "https://b.viktorbarzin.me/_t/e",
            "https://a.viktorbarzin.me/_t/e",
        ),
        (None, None, None),
        (None, "", None),
    ],
)
def test_resolve_host(
    explicit: str | None, env: str | None, expected: str | None, monkeypatch: pytest.MonkeyPatch
) -> None:
    if env is None:
        monkeypatch.delenv("TELEMETRY_HOST", raising=False)
    else:
        monkeypatch.setenv("TELEMETRY_HOST", env)
    assert resolve_host(explicit) == expected


def test_production_needs_a_host(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("TELEMETRY_HOST", raising=False)
    with pytest.raises(ValueError, match="TELEMETRY_HOST"):
        Telemetry(_config())
    with pytest.raises(ValueError, match="TELEMETRY_HOST"):
        telemetry.configure("tv", "wk", "1", "production")


def test_configure_reads_the_host_from_the_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("TELEMETRY_HOST", "https://tv.viktorbarzin.me/_t/e")
    t = telemetry.configure("tv", "wk", "1", "production")
    try:
        client = t.segment_client
        assert client is not None
        assert client.host == "https://tv.viktorbarzin.me/_t/e"
    finally:
        telemetry.configure("tv", "wk", "1", "dev")


def test_dev_needs_no_host(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("TELEMETRY_HOST", raising=False)
    t = telemetry.configure("tv", "wk", "1", "dev")
    assert not t.enabled
    assert t.config.host is None


def test_production_builds_a_segment_client_pointed_at_the_host() -> None:
    t = Telemetry(
        Config(
            app="tv", write_key="wk", release="1", environment="production", host="https://x/_t/e"
        )
    )
    assert t.enabled
    client = t.segment_client
    assert client is not None
    assert client.host == "https://x/_t/e"
    assert client.write_key == "wk"


def test_module_level_functions_use_the_configured_instance() -> None:
    fake = FakeClient()
    t = telemetry.configure("tv", "wk", "1.0.0", "production", client=fake)
    try:
        assert telemetry.track("trip.created", {"n": 1})
        assert telemetry.identify("sub-1")
        telemetry.flush()
        assert [s.kind for s in fake.sent] == ["track", "identify"]
        assert fake.flushes == 1
        assert t.config.app == "tv"
    finally:
        telemetry.configure("tv", "wk", "1.0.0", "dev")


def test_configure_skips_sentry_outside_production(monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[tuple[str, str, str]] = []

    def fake_init(dsn: str, *, release: str, environment: str) -> bool:
        calls.append((dsn, release, environment))
        return True

    monkeypatch.setattr(telemetry._events, "init_sentry", fake_init)
    telemetry.configure("tv", "wk", "1", "dev", dsn="https://k@tv.viktorbarzin.me/_t/x/1")
    assert calls == []
    telemetry.configure(
        "tv",
        "wk",
        "1",
        "production",
        dsn="https://k@tv.viktorbarzin.me/_t/x/1",
        client=FakeClient(),
    )
    assert calls == [("https://k@tv.viktorbarzin.me/_t/x/1", "1", "production")]
    telemetry.configure("tv", "wk", "1", "dev")


def test_configure_registers_the_write_key_for_scrubbing() -> None:
    key = "write-key-0123456789abcdef"
    telemetry.configure("tv", key, "1", "dev")
    fmt = telemetry.JsonFormatter(service_name="tv", service_version="1")
    record = logging.getLogger("segment").makeRecord(
        "segment", logging.DEBUG, "f.py", 1, f"making request: writeKey {key}", (), None
    )
    assert key not in fmt.format(record)
