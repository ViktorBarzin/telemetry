# Typed surface of segment-analytics-python's Client that this package uses.
# The upstream package ships no type information (no py.typed marker).
from collections.abc import Callable, Mapping
from datetime import datetime

class Client:
    write_key: str
    host: str | None
    send: bool
    def __init__(
        self,
        write_key: str,
        host: str | None = ...,
        debug: bool = ...,
        max_queue_size: int = ...,
        send: bool = ...,
        on_error: Callable[[Exception, list[dict[str, object]]], None] | None = ...,
        gzip: bool = ...,
        max_retries: int = ...,
        sync_mode: bool = ...,
        timeout: float = ...,
        thread: int = ...,
        upload_size: int = ...,
        upload_interval: float = ...,
    ) -> None: ...
    def track(
        self,
        user_id: str | None = ...,
        event: str | None = ...,
        properties: Mapping[str, object] | None = ...,
        context: Mapping[str, object] | None = ...,
        timestamp: datetime | None = ...,
        anonymous_id: str | None = ...,
        integrations: Mapping[str, object] | None = ...,
        message_id: str | None = ...,
    ) -> tuple[bool, dict[str, object]]: ...
    def identify(
        self,
        user_id: str | None = ...,
        traits: Mapping[str, object] | None = ...,
        context: Mapping[str, object] | None = ...,
        timestamp: datetime | None = ...,
        anonymous_id: str | None = ...,
        integrations: Mapping[str, object] | None = ...,
        message_id: str | None = ...,
    ) -> tuple[bool, dict[str, object]]: ...
    def flush(self) -> None: ...
    def join(self) -> None: ...
    def shutdown(self) -> None: ...
