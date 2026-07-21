#!/usr/bin/env python3
"""Run one real Dimensio video-task acceptance test through new-api."""

import argparse
import json
import os
import time
from copy import deepcopy
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlsplit
from urllib.request import Request, urlopen
from uuid import uuid4


# Edit only these two values.
BASE_URL = "http://127.0.0.1:3000"
API_KEY = "replace-with-new-api-key"

DEFAULT_MODE = "image"
ACCEPTANCE_MODES = ("text", "image", "multimodal")

PUBLIC_MEDIA_URLS = {
    "image_1": "https://www.w3schools.com/w3images/lights.jpg",
    "image_2": "https://www.w3schools.com/w3images/nature.jpg",
    "image_3": "https://www.w3schools.com/w3images/mountains.jpg",
    "image_4": "https://www.w3schools.com/w3images/forest.jpg",
    "video_1": (
        "https://media.githubusercontent.com/media/adilentiq/"
        "test-images/02f54833d4d08fc33d2090eaeda9f0d2dbf1c7b0/"
        "video/duration/v1_4s.mp4"
    ),
    "audio_1": "https://download.samplelib.com/mp3/sample-3s.mp3",
}

MODE_PAYLOADS = {
    "text": {
        "model": "jimeng-video-seedance-2.0-fast-vip",
        "content": [
            {
                "type": "text",
                "text": (
                    "A calm cinematic shot of morning light moving across a "
                    "modern city skyline, slow camera push forward."
                ),
            }
        ],
        "ratio": "16:9",
        "resolution": "720p",
        "duration": 4,
    },
    "image": {
        "model": "jimeng-video-seedance-2.0-fast-vip",
        "content": [
            {
                "type": "image_url",
                "role": "first_frame",
                "image_url": {"url": PUBLIC_MEDIA_URLS["image_1"]},
            },
            {
                "type": "image_url",
                "role": "last_frame",
                "image_url": {"url": PUBLIC_MEDIA_URLS["image_2"]},
            },
            {
                "type": "text",
                "text": (
                    "Create a smooth cinematic transition from @image_file_1 "
                    "as the first frame to @image_file_2 as the last frame, "
                    "preserve realistic detail and use a slow forward camera "
                    "movement."
                ),
            },
        ],
        "ratio": "16:9",
        "resolution": "720p",
        "duration": 4,
    },
    "multimodal": {
        "model": "jimeng-video-seedance-2.0-fast-vip",
        "content": [
            {
                "type": "image_url",
                "role": "reference_image",
                "image_url": {"url": PUBLIC_MEDIA_URLS["image_1"]},
            },
            {
                "type": "image_url",
                "role": "reference_image",
                "image_url": {"url": PUBLIC_MEDIA_URLS["image_2"]},
            },
            {
                "type": "image_url",
                "role": "reference_image",
                "image_url": {"url": PUBLIC_MEDIA_URLS["image_3"]},
            },
            {
                "type": "image_url",
                "role": "reference_image",
                "image_url": {"url": PUBLIC_MEDIA_URLS["image_4"]},
            },
            {
                "type": "video_url",
                "role": "reference_video",
                "video_url": {"url": PUBLIC_MEDIA_URLS["video_1"]},
            },
            {
                "type": "audio_url",
                "role": "reference_audio",
                "audio_url": {"url": PUBLIC_MEDIA_URLS["audio_1"]},
            },
            {
                "type": "text",
                "text": (
                    "Use @image_file_1 through @image_file_4 as visual "
                    "references, follow the motion from @video_file_1 and the "
                    "rhythm from @audio_file_1, then create a coherent "
                    "cinematic scene with a slow forward camera movement."
                ),
            },
        ],
        "ratio": "16:9",
        "resolution": "720p",
        "duration": 4,
    },
}

OUTPUT_ROOT = Path("output") / "dimensio-client-acceptance"
POLL_INTERVAL_SECONDS = 5
MAX_WAIT_SECONDS = 15 * 60
HTTP_TIMEOUT_SECONDS = 30
SUBMIT_TIMEOUT_SECONDS = 120
DOWNLOAD_TIMEOUT_SECONDS = 120
DOWNLOAD_MAX_ATTEMPTS = 3
DOWNLOAD_RETRY_DELAY_SECONDS = 1
ASSET_TIMEOUT_SECONDS = 20
CHUNK_SIZE = 64 * 1024


class AcceptanceError(Exception):
    def __init__(
        self, category: str, message: str, details: dict[str, Any] | None = None
    ) -> None:
        super().__init__(message)
        self.category = category
        self.details = details or {}


def build_payload(mode: str) -> dict[str, Any]:
    payload = MODE_PAYLOADS.get(mode)
    if payload is None:
        raise AcceptanceError(
            "config",
            "mode must be one of: " + ", ".join(ACCEPTANCE_MODES),
            {"mode": mode},
        )
    return deepcopy(payload)


def _parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run one real Dimensio video acceptance task through new-api."
    )
    parser.add_argument(
        "--mode",
        choices=ACCEPTANCE_MODES,
        default=DEFAULT_MODE,
        help="video generation mode (default: image)",
    )
    return parser.parse_args(argv)


def _iso_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def _api_key_hint(api_key: str) -> str:
    key = api_key.strip()
    return f"***{key[-4:]}" if len(key) > 4 else "***"


def _redact(value: Any, secret: str) -> Any:
    if isinstance(value, dict):
        return {key: _redact(item, secret) for key, item in value.items()}
    if isinstance(value, list):
        return [_redact(item, secret) for item in value]
    if isinstance(value, str) and secret:
        return value.replace(secret, "[REDACTED]")
    return value


def _create_run_dir(output_root: Path) -> Path:
    run_id = (
        datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S.%fZ")
        + "-"
        + uuid4().hex[:8]
    )
    run_dir = output_root / run_id
    run_dir.mkdir(parents=True, exist_ok=False)
    return run_dir


def _validate_config(base_url: str, api_key: str) -> tuple[str, str]:
    normalized_base_url = base_url.strip().rstrip("/")
    parsed = urlsplit(normalized_base_url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise AcceptanceError(
            "config", "BASE_URL must be a complete HTTP(S) URL"
        )
    if parsed.query or parsed.fragment:
        raise AcceptanceError(
            "config", "BASE_URL must not contain a query string or fragment"
        )

    normalized_api_key = api_key.strip()
    if (
        not normalized_api_key
        or normalized_api_key.lower() == "replace-with-new-api-key"
    ):
        raise AcceptanceError(
            "config", "API_KEY must be replaced with a new-api console token"
        )
    return normalized_base_url, normalized_api_key


def _extract_assets(payload: dict[str, Any]) -> list[dict[str, str]]:
    assets: list[dict[str, str]] = []
    fields = {
        "image_url": ("image", "image_url"),
        "video_url": ("video", "video_url"),
        "audio_url": ("audio", "audio_url"),
    }
    for item in payload.get("content", []):
        if not isinstance(item, dict):
            continue
        field = item.get("type")
        if field not in fields:
            continue
        asset_type, container_name = fields[field]
        container = item.get(container_name)
        url = container.get("url") if isinstance(container, dict) else None
        if not isinstance(url, str) or not url.strip():
            raise AcceptanceError(
                "asset",
                f"{field}.url is required",
                {"type": asset_type, "role": item.get("role", "")},
            )
        assets.append(
            {
                "type": asset_type,
                "role": str(item.get("role") or ""),
                "url": url.strip(),
            }
        )
    return assets


def _preflight_assets(
    payload: dict[str, Any],
) -> list[dict[str, Any]]:
    checks: list[dict[str, Any]] = []
    for asset in _extract_assets(payload):
        url = asset["url"]
        parsed = urlsplit(url)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc:
            raise AcceptanceError(
                "asset",
                f"{asset['type']} asset must use a complete HTTP(S) URL",
                {"asset": asset},
            )

        headers = {
            "Accept": (
                "image/*"
                if asset["type"] == "image"
                else "audio/*"
                if asset["type"] == "audio"
                else "video/mp4,application/octet-stream"
            ),
            "User-Agent": "new-api-dimensio-acceptance/1.0",
        }
        method = "HEAD"
        if asset["type"] == "video":
            if not parsed.path.lower().endswith(".mp4"):
                raise AcceptanceError(
                    "asset",
                    "reference video URL must end with .mp4",
                    {"asset": asset},
                )
            method = "GET"
            headers["Range"] = "bytes=0-11"

        request = Request(url, headers=headers, method=method)
        try:
            with urlopen(
                request, timeout=ASSET_TIMEOUT_SECONDS
            ) as response:
                status = getattr(response, "status", 200)
                content_type = (
                    response.headers.get("Content-Type", "")
                    .split(";", 1)[0]
                    .strip()
                    .lower()
                )
                if status < 200 or status >= 300:
                    raise AcceptanceError(
                        "asset",
                        (
                            f"{asset['type']} asset returned "
                            f"HTTP {status}"
                        ),
                        {"asset": asset, "http_status": status},
                    )
                header = (
                    response.read(12)
                    if asset["type"] == "video"
                    else b""
                )
        except HTTPError as exc:
            raise AcceptanceError(
                "asset",
                f"{asset['type']} asset returned HTTP {exc.code}",
                {"asset": asset, "http_status": exc.code},
            ) from exc
        except (URLError, TimeoutError, OSError) as exc:
            raise AcceptanceError(
                "asset",
                f"{asset['type']} asset check failed: {exc}",
                {"asset": asset},
            ) from exc

        expected_prefix = {
            "image": "image/",
            "audio": "audio/",
        }.get(asset["type"])
        if expected_prefix and not content_type.startswith(expected_prefix):
            raise AcceptanceError(
                "asset",
                (
                    f"{asset['type']} asset returned unexpected "
                    f"Content-Type: {content_type or 'missing'}"
                ),
                {
                    "asset": asset,
                    "http_status": status,
                    "content_type": content_type,
                },
            )
        if asset["type"] == "video" and (
            len(header) < 8 or header[4:8] != b"ftyp"
        ):
            raise AcceptanceError(
                "asset",
                "reference video does not have a valid MP4 ftyp box",
                {
                    "asset": asset,
                    "http_status": status,
                    "content_type": content_type,
                },
            )

        checks.append(
            {
                **asset,
                "ok": True,
                "http_status": status,
                "content_type": content_type,
                "mp4_ftyp": (
                    True if asset["type"] == "video" else None
                ),
            }
        )
    return checks


def _extract_error(payload: object) -> tuple[str, str]:
    if not isinstance(payload, dict):
        return "UNKNOWN", "Upstream returned an unstructured error"

    nested = payload.get("error")
    if isinstance(nested, dict):
        code = str(nested.get("code") or "UNKNOWN")
        message = str(nested.get("message") or "Upstream request failed")
        return code, message

    code = str(payload.get("code") or "UNKNOWN")
    message = str(payload.get("message") or "Upstream request failed")
    return code, message


def _request_json(
    method: str,
    url: str,
    api_key: str,
    payload: dict[str, Any] | None = None,
    timeout_seconds: float = HTTP_TIMEOUT_SECONDS,
) -> dict[str, Any]:
    body = None
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Accept": "application/json",
    }
    if payload is not None:
        body = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"

    request = Request(url, data=body, headers=headers, method=method)
    try:
        with urlopen(request, timeout=timeout_seconds) as response:
            raw_body = response.read()
    except HTTPError as exc:
        raw_body = exc.read()
        try:
            response_payload: object = json.loads(raw_body.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError):
            response_payload = None
        code, message = _extract_error(response_payload)
        details: dict[str, Any] = {
            "http_status": exc.code,
            "upstream_error": {"code": code, "message": message},
        }
        if response_payload is not None:
            details["response"] = response_payload
        else:
            details["response_text"] = raw_body.decode("utf-8", errors="replace")
        raise AcceptanceError(
            "http", f"HTTP {exc.code}: {code}: {message}", details
        ) from exc
    except (URLError, TimeoutError, OSError) as exc:
        raise AcceptanceError("network", f"Gateway request failed: {exc}") from exc

    try:
        decoded = json.loads(raw_body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise AcceptanceError(
            "protocol", "Gateway returned a non-JSON success response"
        ) from exc
    if not isinstance(decoded, dict):
        raise AcceptanceError(
            "protocol", "Gateway JSON response must be an object"
        )
    return decoded


def _download_video(video_url: str, target: Path) -> None:
    parsed = urlsplit(video_url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise AcceptanceError(
            "download", "content.video_url must be a complete HTTP(S) URL"
        )

    partial = target.with_suffix(target.suffix + ".part")
    request = Request(
        video_url,
        headers={
            "Accept": "video/*",
            "User-Agent": "new-api-dimensio-acceptance/1.0",
        },
        method="GET",
    )
    for attempt in range(1, DOWNLOAD_MAX_ATTEMPTS + 1):
        partial.unlink(missing_ok=True)
        try:
            with urlopen(request, timeout=DOWNLOAD_TIMEOUT_SECONDS) as response:
                with partial.open("wb") as output:
                    total_bytes = 0
                    while True:
                        chunk = response.read(CHUNK_SIZE)
                        if not chunk:
                            break
                        output.write(chunk)
                        total_bytes += len(chunk)
            if total_bytes == 0:
                raise AcceptanceError("download", "Downloaded video is empty")
            os.replace(partial, target)
            return
        except AcceptanceError:
            partial.unlink(missing_ok=True)
            raise
        except HTTPError as exc:
            partial.unlink(missing_ok=True)
            raise AcceptanceError(
                "download", f"Video download returned HTTP {exc.code}"
            ) from exc
        except (URLError, TimeoutError, OSError) as exc:
            partial.unlink(missing_ok=True)
            if attempt == DOWNLOAD_MAX_ATTEMPTS:
                raise AcceptanceError(
                    "download", f"Video download failed: {exc}"
                ) from exc
            time.sleep(DOWNLOAD_RETRY_DELAY_SECONDS * attempt)


def _write_report(report: dict[str, Any], path: Path, api_key: str) -> None:
    sanitized = _redact(report, api_key.strip())
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(
        json.dumps(sanitized, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    os.replace(temporary, path)


def run_acceptance(
    *,
    mode: str = DEFAULT_MODE,
    base_url: str = BASE_URL,
    api_key: str = API_KEY,
    output_root: Path = OUTPUT_ROOT,
    poll_interval_seconds: float = POLL_INTERVAL_SECONDS,
    max_wait_seconds: float = MAX_WAIT_SECONDS,
) -> tuple[int, Path]:
    run_dir = _create_run_dir(output_root)
    started_at = _iso_now()
    started_monotonic = time.monotonic()
    report: dict[str, Any] = {
        "result": "failed",
        "api_key_hint": _api_key_hint(api_key),
        "mode": mode,
        "request": {
            "base_url": base_url,
            "payload": None,
        },
        "assets": [],
        "asset_checks": [],
        "task_id": None,
        "status_history": [],
        "submit_response": None,
        "final_response": None,
        "video_url": None,
        "video_path": None,
        "started_at": started_at,
        "finished_at": None,
        "elapsed_seconds": None,
        "error": None,
    }
    exit_code = 1

    try:
        normalized_base_url, normalized_api_key = _validate_config(
            base_url, api_key
        )
        payload = build_payload(mode)
        report["request"]["payload"] = payload
        report["assets"] = _extract_assets(payload)
        report["asset_checks"] = _preflight_assets(payload)
        submit_url = (
            normalized_base_url + "/api/v3/contents/generations/tasks"
        )
        report["request"]["base_url"] = normalized_base_url
        report["request"]["submit_url"] = submit_url

        submit_response = _request_json(
            "POST",
            submit_url,
            normalized_api_key,
            payload,
            timeout_seconds=SUBMIT_TIMEOUT_SECONDS,
        )
        report["submit_response"] = submit_response
        task_id = submit_response.get("id")
        if not isinstance(task_id, str) or not task_id.strip():
            raise AcceptanceError(
                "protocol", "Submit response does not contain a public task id"
            )
        task_id = task_id.strip()
        report["task_id"] = task_id

        poll_url = (
            normalized_base_url
            + "/api/v3/contents/generations/tasks/"
            + quote(task_id, safe="")
        )
        report["request"]["poll_url"] = poll_url
        poll_started = time.monotonic()

        while True:
            final_response = _request_json(
                "GET", poll_url, normalized_api_key
            )
            report["final_response"] = final_response
            status = final_response.get("status")
            if not isinstance(status, str):
                raise AcceptanceError(
                    "protocol", "Task response does not contain a string status"
                )
            report["status_history"].append(
                {"status": status, "observed_at": _iso_now()}
            )

            if status in {"queued", "running"}:
                elapsed = time.monotonic() - poll_started
                if elapsed >= max_wait_seconds:
                    raise AcceptanceError(
                        "timeout",
                        f"Task did not finish within {max_wait_seconds:g} seconds",
                        {"max_wait_seconds": max_wait_seconds},
                    )
                time.sleep(
                    min(poll_interval_seconds, max_wait_seconds - elapsed)
                )
                continue

            if status == "failed":
                code, message = _extract_error(final_response)
                raise AcceptanceError(
                    "task",
                    f"Task failed: {code}: {message}",
                    {
                        "upstream_error": {
                            "code": code,
                            "message": message,
                        }
                    },
                )

            if status != "succeeded":
                raise AcceptanceError(
                    "protocol", f"Task returned unknown status: {status}"
                )

            content = final_response.get("content")
            video_url = (
                content.get("video_url") if isinstance(content, dict) else None
            )
            if not isinstance(video_url, str) or not video_url.strip():
                raise AcceptanceError(
                    "protocol",
                    "Succeeded task does not contain content.video_url",
                )
            report["video_url"] = video_url.strip()
            break

        video_path = run_dir / "video.mp4"
        _download_video(report["video_url"], video_path)
        report["video_path"] = str(video_path.resolve())
        report["result"] = "passed"
        exit_code = 0
    except KeyboardInterrupt:
        report["result"] = "interrupted"
        report["error"] = {
            "category": "interrupt",
            "message": "Interrupted by user",
            "details": {},
        }
        exit_code = 130
    except AcceptanceError as exc:
        report["error"] = {
            "category": exc.category,
            "message": str(exc),
            "details": exc.details,
        }
    except Exception as exc:
        report["error"] = {
            "category": "internal",
            "message": f"Unexpected client error: {exc}",
            "details": {},
        }
    finally:
        report["finished_at"] = _iso_now()
        report["elapsed_seconds"] = round(
            time.monotonic() - started_monotonic, 3
        )
        try:
            _write_report(report, run_dir / "report.json", api_key)
        except OSError as exc:
            print(f"Could not write report: {exc}")
            exit_code = 1

    outcome = "passed" if exit_code == 0 else "did not pass"
    print(f"Dimensio acceptance {outcome}. Artifacts: {run_dir.resolve()}")
    return exit_code, run_dir


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    exit_code, _ = run_acceptance(mode=args.mode)
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
