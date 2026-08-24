#!/usr/bin/env python3
"""Fetch and validate the public Dimensio model catalog."""

from __future__ import annotations

import argparse
import copy
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import urlparse
from urllib.request import Request, urlopen


DEFAULT_BASE_URL = "https://jimeng.dimensio.cn"
MODELS_PATH = "/v1/models"
DEFAULT_TIMEOUT_SECONDS = 20
MAX_TIMEOUT_SECONDS = 60
MAX_RESPONSE_BYTES = 10 * 1024 * 1024


def _models_url(base_url: str) -> str:
    normalized = base_url.strip().rstrip("/")
    parsed = urlparse(normalized)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("base_url must be an absolute http(s) URL")
    return normalized + MODELS_PATH


def parse_models_payload(payload: Any) -> list[dict[str, Any]]:
    if not isinstance(payload, dict):
        raise ValueError("Dimensio models response must be a JSON object")

    data = payload.get("data")
    if not isinstance(data, list) or not data:
        raise ValueError("Dimensio models response data must be a non-empty array")

    models: list[dict[str, Any]] = []
    seen_ids: set[str] = set()
    for entry in data:
        if not isinstance(entry, dict):
            raise ValueError("Dimensio model entry must be a JSON object")
        model_id = entry.get("id")
        if not isinstance(model_id, str) or not model_id.strip():
            raise ValueError("Dimensio model entry id must be a non-empty string")
        model_id = model_id.strip()
        if model_id in seen_ids:
            raise ValueError(f"duplicate model id: {model_id}")
        seen_ids.add(model_id)

        normalized = copy.deepcopy(entry)
        normalized["id"] = model_id
        models.append(normalized)
    return models


def fetch_models(base_url: str = DEFAULT_BASE_URL, timeout: int = DEFAULT_TIMEOUT_SECONDS) -> list[dict[str, Any]]:
    if timeout <= 0 or timeout > MAX_TIMEOUT_SECONDS:
        raise ValueError(f"timeout must be between 1 and {MAX_TIMEOUT_SECONDS} seconds")

    request_url = _models_url(base_url)
    request = Request(
        request_url,
        headers={"Accept": "application/json", "User-Agent": "new-api-dimensio-model-sync/1"},
        method="GET",
    )
    try:
        with urlopen(request, timeout=timeout) as response:
            status = int(response.status)
            if status < 200 or status >= 300:
                raise RuntimeError(f"Dimensio models request failed with status {status}")
            body = response.read(MAX_RESPONSE_BYTES + 1)
    except HTTPError as error:
        raise RuntimeError(f"Dimensio models request failed with status {error.code}") from error
    except URLError as error:
        raise RuntimeError(f"Dimensio models request failed: {error.reason}") from error

    if len(body) > MAX_RESPONSE_BYTES:
        raise RuntimeError("Dimensio models response exceeds the 10 MiB limit")
    try:
        payload = json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise RuntimeError("Dimensio models response is not valid UTF-8 JSON") from error
    return parse_models_payload(payload)


def build_snapshot(
    models: list[dict[str, Any]],
    source_url: str,
    fetched_at: str,
) -> dict[str, Any]:
    normalized_models = parse_models_payload({"data": models})
    canonical = json.dumps(
        normalized_models,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    ).encode("utf-8")
    return {
        "schema_version": 1,
        "source_url": source_url,
        "fetched_at": fetched_at,
        "model_count": len(normalized_models),
        "models_sha256": hashlib.sha256(canonical).hexdigest(),
        "models": normalized_models,
    }


def _parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default=DEFAULT_BASE_URL)
    parser.add_argument("--timeout", type=int, default=DEFAULT_TIMEOUT_SECONDS)
    parser.add_argument("--output", type=Path, help="write the JSON snapshot to this path")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(sys.argv[1:] if argv is None else argv)
    source_url = _models_url(args.base_url)
    models = fetch_models(args.base_url, args.timeout)
    snapshot = build_snapshot(
        models,
        source_url=source_url,
        fetched_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
    )
    rendered = json.dumps(snapshot, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(rendered, encoding="utf-8")
    else:
        sys.stdout.write(rendered)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (RuntimeError, ValueError) as error:
        print(f"error: {error}", file=sys.stderr)
        raise SystemExit(1)
