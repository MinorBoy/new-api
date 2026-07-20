# Dimensio Client Acceptance Script Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Python standard-library client that validates a real Dimensio task through the new-api ARK v3 gateway, downloads the resulting video, and writes a redacted JSON acceptance report.

**Architecture:** Keep the runnable client and its tests in `scripts/`. The client owns configuration validation, authenticated gateway requests, bounded polling, unauthenticated external video download, and atomic artifacts; tests exercise the public `run_acceptance` entry point through a local `ThreadingHTTPServer` so the complete wire contract is covered without calling an upstream provider.

**Tech Stack:** Python 3.10+ standard library (`urllib.request`, `http.server`, `unittest`), ARK v3 task API

---

## File Map

- Create `scripts/dimensio_acceptance.py`: user configuration, ARK task lifecycle, video download, report generation, and CLI exit codes.
- Create `scripts/test_dimensio_acceptance.py`: deterministic local HTTP server and end-to-end client contract tests.

### Task 1: Lock the client contract with failing local-server tests

**Files:**
- Create: `scripts/test_dimensio_acceptance.py`
- Test: `scripts/test_dimensio_acceptance.py`

- [ ] **Step 1: Write the complete failing test module**

Create `scripts/test_dimensio_acceptance.py` with:

```python
import json
import threading
import unittest
from contextlib import contextmanager
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from tempfile import TemporaryDirectory
from unittest.mock import patch
from urllib.parse import urlsplit

from scripts import dimensio_acceptance as acceptance


@dataclass
class MockState:
    submit_status: int = 200
    submit_body: dict = field(default_factory=lambda: {"id": "public-task-1"})
    polls: list[tuple[int, dict]] = field(default_factory=list)
    video_body: bytes = b"mock-mp4-bytes"
    gateway_requests: list[dict] = field(default_factory=list)
    video_requests: list[dict] = field(default_factory=list)


class MockHandler(BaseHTTPRequestHandler):
    def _headers(self) -> dict[str, str]:
        return {key.lower(): value for key, value in self.headers.items()}

    def _send_json(self, status: int, payload: dict) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self) -> None:
        state = self.server.state
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length)
        state.gateway_requests.append(
            {
                "method": "POST",
                "path": urlsplit(self.path).path,
                "headers": self._headers(),
                "json": json.loads(body.decode("utf-8")),
            }
        )
        self._send_json(state.submit_status, state.submit_body)

    def do_GET(self) -> None:
        state = self.server.state
        path = urlsplit(self.path).path
        request = {
            "method": "GET",
            "path": path,
            "headers": self._headers(),
        }
        if path == "/video.mp4":
            state.video_requests.append(request)
            self.send_response(200)
            self.send_header("Content-Type", "video/mp4")
            self.send_header("Content-Length", str(len(state.video_body)))
            self.end_headers()
            self.wfile.write(state.video_body)
            return

        state.gateway_requests.append(request)
        if not state.polls:
            self._send_json(
                500,
                {"error": {"code": "MOCK_EXHAUSTED", "message": "No poll left"}},
            )
            return
        status, payload = state.polls.pop(0)
        self._send_json(status, payload)

    def log_message(self, format: str, *args: object) -> None:
        return


@contextmanager
def serve(state: MockState):
    server = ThreadingHTTPServer(("127.0.0.1", 0), MockHandler)
    server.daemon_threads = True
    server.state = state
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        host, port = server.server_address
        yield f"http://{host}:{port}"
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


class DimensioAcceptanceTest(unittest.TestCase):
    def read_report(self, run_dir: Path) -> tuple[dict, str]:
        report_text = (run_dir / "report.json").read_text(encoding="utf-8")
        return json.loads(report_text), report_text

    def test_successful_lifecycle_downloads_video_and_redacts_key(self) -> None:
        state = MockState()
        api_key = "test-secret-1234"
        with TemporaryDirectory() as temp_dir, serve(state) as base_url:
            state.polls = [
                (200, {"id": "public-task-1", "status": "queued"}),
                (200, {"id": "public-task-1", "status": "running"}),
                (
                    200,
                    {
                        "id": "public-task-1",
                        "status": "succeeded",
                        "content": {"video_url": f"{base_url}/video.mp4"},
                    },
                ),
            ]

            exit_code, run_dir = acceptance.run_acceptance(
                base_url=base_url,
                api_key=api_key,
                output_root=Path(temp_dir),
                poll_interval_seconds=0,
                max_wait_seconds=5,
            )

            self.assertEqual(exit_code, 0)
            self.assertEqual((run_dir / "video.mp4").read_bytes(), state.video_body)
            self.assertEqual(
                [request["path"] for request in state.gateway_requests],
                [
                    "/api/v3/contents/generations/tasks",
                    "/api/v3/contents/generations/tasks/public-task-1",
                    "/api/v3/contents/generations/tasks/public-task-1",
                    "/api/v3/contents/generations/tasks/public-task-1",
                ],
            )
            self.assertEqual(
                state.gateway_requests[0]["json"], acceptance.FIXED_PAYLOAD
            )
            for request in state.gateway_requests:
                self.assertEqual(
                    request["headers"]["authorization"], f"Bearer {api_key}"
                )
                self.assertEqual(request["headers"]["accept"], "application/json")
            self.assertEqual(
                state.gateway_requests[0]["headers"]["content-type"],
                "application/json",
            )
            self.assertEqual(len(state.video_requests), 1)
            self.assertNotIn(
                "authorization", state.video_requests[0]["headers"]
            )

            report, report_text = self.read_report(run_dir)
            self.assertEqual(report["result"], "passed")
            self.assertEqual(report["api_key_hint"], "***1234")
            self.assertEqual(report["task_id"], "public-task-1")
            self.assertEqual(
                [item["status"] for item in report["status_history"]],
                ["queued", "running", "succeeded"],
            )
            self.assertEqual(report["request"]["payload"], acceptance.FIXED_PAYLOAD)
            self.assertEqual(
                Path(report["video_path"]).read_bytes(), state.video_body
            )
            self.assertNotIn(api_key, report_text)

    def test_failed_task_reports_error_and_does_not_download(self) -> None:
        state = MockState(
            polls=[
                (
                    200,
                    {
                        "id": "public-task-1",
                        "status": "failed",
                        "error": {
                            "code": "DIMENSIO_REJECTED",
                            "message": "Prompt rejected",
                        },
                    },
                )
            ]
        )
        with TemporaryDirectory() as temp_dir, serve(state) as base_url:
            exit_code, run_dir = acceptance.run_acceptance(
                base_url=base_url,
                api_key="test-secret-5678",
                output_root=Path(temp_dir),
                poll_interval_seconds=0,
                max_wait_seconds=5,
            )

            self.assertEqual(exit_code, 1)
            self.assertFalse((run_dir / "video.mp4").exists())
            self.assertEqual(state.video_requests, [])
            report, _ = self.read_report(run_dir)
            self.assertEqual(report["result"], "failed")
            self.assertEqual(report["error"]["category"], "task")
            self.assertEqual(
                report["error"]["details"]["upstream_error"],
                {"code": "DIMENSIO_REJECTED", "message": "Prompt rejected"},
            )
            self.assertEqual(report["final_response"]["status"], "failed")

    def test_non_2xx_ark_and_top_level_errors_are_preserved(self) -> None:
        cases = [
            (
                "ark",
                {"error": {"code": "RATE_LIMITED", "message": "Slow down"}},
                {"code": "RATE_LIMITED", "message": "Slow down"},
            ),
            (
                "top_level",
                {"code": "UNAUTHORIZED", "message": "Bad token"},
                {"code": "UNAUTHORIZED", "message": "Bad token"},
            ),
        ]
        for label, response, expected_error in cases:
            with self.subTest(label=label), TemporaryDirectory() as temp_dir:
                state = MockState(submit_status=429, submit_body=response)
                with serve(state) as base_url:
                    exit_code, run_dir = acceptance.run_acceptance(
                        base_url=base_url,
                        api_key="test-secret-9012",
                        output_root=Path(temp_dir),
                        poll_interval_seconds=0,
                        max_wait_seconds=5,
                    )

                self.assertEqual(exit_code, 1)
                report, _ = self.read_report(run_dir)
                self.assertEqual(report["error"]["category"], "http")
                self.assertEqual(report["error"]["details"]["http_status"], 429)
                self.assertEqual(
                    report["error"]["details"]["upstream_error"], expected_error
                )
                self.assertEqual(
                    report["error"]["details"]["response"], response
                )

    def test_invalid_configuration_fails_before_network_and_writes_report(self) -> None:
        cases = [
            ("invalid_url", "not-a-url", "real-secret-1234"),
            (
                "placeholder_key",
                "http://127.0.0.1:3000",
                "replace-with-new-api-key",
            ),
        ]
        with TemporaryDirectory() as temp_dir, patch.object(
            acceptance, "_request_json"
        ) as request_json:
            for label, base_url, api_key in cases:
                with self.subTest(label=label):
                    exit_code, run_dir = acceptance.run_acceptance(
                        base_url=base_url,
                        api_key=api_key,
                        output_root=Path(temp_dir) / label,
                        poll_interval_seconds=0,
                        max_wait_seconds=5,
                    )
                    self.assertEqual(exit_code, 1)
                    report, report_text = self.read_report(run_dir)
                    self.assertEqual(report["result"], "failed")
                    self.assertEqual(report["error"]["category"], "config")
                    self.assertNotIn(api_key, report_text)

            request_json.assert_not_called()


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run the test and verify RED**

Run from the repository root:

```bash
python -m unittest scripts.test_dimensio_acceptance -v
```

Expected: FAIL during import with `ImportError` because `scripts/dimensio_acceptance.py` does not exist yet. Do not weaken the assertions to make the test load.

### Task 2: Implement the standard-library acceptance client

**Files:**
- Create: `scripts/dimensio_acceptance.py`
- Test: `scripts/test_dimensio_acceptance.py`

- [ ] **Step 1: Add the complete minimal implementation**

Create `scripts/dimensio_acceptance.py` with:

```python
#!/usr/bin/env python3
"""Run one real Dimensio video-task acceptance test through new-api."""

import json
import os
import time
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

FIXED_PAYLOAD = {
    "model": "jimeng-video-seedance-2.0-fast-vip",
    "content": [
        {
            "type": "text",
            "text": (
                "A calm cinematic shot of morning light moving across a modern "
                "city skyline, slow camera push forward."
            ),
        }
    ],
    "ratio": "16:9",
    "resolution": "720p",
    "duration": 4,
}

OUTPUT_ROOT = Path("output") / "dimensio-client-acceptance"
POLL_INTERVAL_SECONDS = 5
MAX_WAIT_SECONDS = 15 * 60
HTTP_TIMEOUT_SECONDS = 30
DOWNLOAD_TIMEOUT_SECONDS = 120
CHUNK_SIZE = 64 * 1024


class AcceptanceError(Exception):
    def __init__(
        self, category: str, message: str, details: dict[str, Any] | None = None
    ) -> None:
        super().__init__(message)
        self.category = category
        self.details = details or {}


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
        with urlopen(request, timeout=HTTP_TIMEOUT_SECONDS) as response:
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
    partial.unlink(missing_ok=True)
    request = Request(
        video_url,
        headers={
            "Accept": "video/*",
            "User-Agent": "new-api-dimensio-acceptance/1.0",
        },
        method="GET",
    )
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
        raise AcceptanceError("download", f"Video download failed: {exc}") from exc


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
        "request": {
            "base_url": base_url,
            "payload": FIXED_PAYLOAD,
        },
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
        submit_url = (
            normalized_base_url + "/api/v3/contents/generations/tasks"
        )
        report["request"]["base_url"] = normalized_base_url
        report["request"]["submit_url"] = submit_url

        submit_response = _request_json(
            "POST", submit_url, normalized_api_key, FIXED_PAYLOAD
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


def main() -> int:
    exit_code, _ = run_acceptance()
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
```

The download request deliberately contains no `Authorization` header. Keep this boundary intact even if a future video host happens to share the gateway hostname.

- [ ] **Step 2: Run the focused suite and verify GREEN**

Run from the repository root:

```bash
python -m unittest scripts.test_dimensio_acceptance -v
```

Expected: 4 tests pass, including two non-2xx subcases. The output must contain `OK`.

- [ ] **Step 3: Commit the tested client**

```bash
git add scripts/dimensio_acceptance.py scripts/test_dimensio_acceptance.py
git commit -m "test(dimensio): add client acceptance script"
```

Expected: one commit containing only the client and its test module.

### Task 3: Verify CLI behavior and artifact safety

**Files:**
- Verify: `scripts/dimensio_acceptance.py`
- Verify: `scripts/test_dimensio_acceptance.py`

- [ ] **Step 1: Compile both Python modules**

Run from the repository root:

```bash
python -m py_compile scripts/dimensio_acceptance.py scripts/test_dimensio_acceptance.py
```

Expected: exit code 0 with no output.

- [ ] **Step 2: Run the complete deterministic acceptance suite again**

```bash
python -m unittest scripts.test_dimensio_acceptance -v
```

Expected: all 4 tests pass. This verifies the submit/poll paths, fixed payload, gateway authorization, absent download authorization, state history, video bytes, nested and top-level errors, pre-network configuration failures, and report redaction.

- [ ] **Step 3: Exercise the unconfigured CLI failure path**

Before setting real credentials, leave the two file-top defaults unchanged and run:

```bash
python scripts/dimensio_acceptance.py
```

Expected: exit code 1, a new `output/dimensio-client-acceptance/<run-id>/report.json`, no network request, no `video.mp4`, and a report whose `error.category` is `config`. The console prints only the artifact directory and never the full placeholder or a future real API key.

- [ ] **Step 4: Inspect repository scope**

```bash
git status --short
git diff --check
```

Expected: no whitespace errors and no changes outside the two planned script files (apart from pre-existing user-owned work). Do not stage generated `output/` artifacts.

### Task 4: Run the real Dimensio acceptance

**Files:**
- Modify locally: `scripts/dimensio_acceptance.py` (only the two configuration values; do not commit secrets)
- Produce: `output/dimensio-client-acceptance/<run-id>/video.mp4`
- Produce: `output/dimensio-client-acceptance/<run-id>/report.json`

- [ ] **Step 1: Configure only the gateway URL and console token**

Set:

```python
BASE_URL = "http://127.0.0.1:3000"
API_KEY = "replace-with-the-real-new-api-console-token"
```

`BASE_URL` must be the new-api gateway, not `https://jimeng.dimensio.cn`. Use a new-api console API token that can route model `jimeng-video-seedance-2.0-fast-vip` to the configured Dimensio channel.

- [ ] **Step 2: Run the real acceptance**

```bash
python scripts/dimensio_acceptance.py
```

Expected: the process polls at 5-second intervals for at most 15 minutes and exits 0 after downloading the video.

- [ ] **Step 3: Validate the real artifacts**

Open the printed run directory and verify:

- `video.mp4` exists, is non-empty, and plays.
- `report.json` has `"result": "passed"`, a public `task_id`, a terminal `succeeded` status, the fixed request payload, and the local video path.
- `api_key_hint` is `***<last4>` and the complete API key does not appear anywhere in `report.json` or console output.

- [ ] **Step 4: Remove the real token before any later commit**

Restore only the two configuration lines to:

```python
BASE_URL = "http://127.0.0.1:3000"
API_KEY = "replace-with-new-api-key"
```

Run `git diff -- scripts/dimensio_acceptance.py` and confirm no credential-bearing diff remains. Preserve the generated report and video as local acceptance evidence; do not stage them.
