"""Redact secrets from a rendered log line before it leaves the process.

The same approach as tripit's ``observability.Scrubber``: known secret values,
plus credential-shaped patterns that should never reach a shared log store.
"""

from __future__ import annotations

import re
from collections.abc import Iterable

REDACTED = "[REDACTED]"

# Values shorter than this are not masked: hiding short config such as "dev"
# would remove ordinary content from the logs.
MIN_SECRET_LENGTH = 8

_BEARER_RE = re.compile(r"(?i)\bbearer\s+[A-Za-z0-9._~+/=\-]+")
_BASIC_RE = re.compile(r"(?i)\bbasic\s+[A-Za-z0-9+/]{8,}={0,2}")
_JWT_RE = re.compile(r"\beyJ[A-Za-z0-9_\-]{5,}\.[A-Za-z0-9_\-]{5,}\.[A-Za-z0-9_\-]{2,}")
_PEM_RE = re.compile(r"-----BEGIN[^-]+-----.*?-----END[^-]+-----", re.DOTALL)


_registered: tuple[str, ...] = ()


def _longest_first(values: Iterable[str]) -> tuple[str, ...]:
    # Longest first, so a secret that contains another is masked whole.
    return tuple(sorted({v for v in values if len(v) >= MIN_SECRET_LENGTH}, key=len, reverse=True))


def register_secret(value: str) -> None:
    """Mask ``value`` in every Scrubber from now on. ``configure`` registers the write key."""
    global _registered
    _registered = _longest_first((*_registered, value))


class Scrubber:
    def __init__(self, secret_values: Iterable[str] = ()) -> None:
        self._own = _longest_first(secret_values)
        self._seen: tuple[str, ...] | None = None
        self._secrets: tuple[str, ...] = self._own

    def scrub(self, text: str) -> str:
        if self._seen is not _registered:
            self._seen = _registered
            self._secrets = _longest_first((*self._own, *_registered))
        for secret in self._secrets:
            if secret in text:
                text = text.replace(secret, REDACTED)
        text = _BEARER_RE.sub(f"Bearer {REDACTED}", text)
        text = _BASIC_RE.sub(f"Basic {REDACTED}", text)
        text = _JWT_RE.sub(REDACTED, text)
        return _PEM_RE.sub(REDACTED, text)
