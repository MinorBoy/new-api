# Dimensio Acceptance Modes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the standard-library Dimensio acceptance client with selectable text, two-frame image, and six-asset multimodal video-generation modes, defaulting to image mode.

**Architecture:** Keep the client in one runnable module and represent the three user-facing modes as immutable payload templates copied per run. Add a separate anonymous media-preflight boundary before gateway submission, then reuse the existing submit, poll, download, redaction, and report lifecycle for every mode.

**Tech Stack:** Python 3.10+ standard library (`argparse`, `copy`, `urllib.request`, `http.server`, `unittest`), ARK v3 task API

---

## File Map

- Modify `scripts/dimensio_acceptance.py`: mode registry, CLI parsing, fixed public assets, anonymous media preflight, selected payload submission, and report fields.
- Modify `scripts/test_dimensio_acceptance.py`: payload/CLI tests, local asset server behavior, media-preflight failures, and three-mode lifecycle coverage.
- Reference `docs/superpowers/specs/2026-07-21-dimensio-acceptance-modes-design.md`: approved behavioral contract and exact public asset URLs.

### Task 1: Lock the mode registry and CLI contract with failing tests

**Files:**
- Modify: `scripts/test_dimensio_acceptance.py:1-14`
- Modify: `scripts/test_dimensio_acceptance.py:98`
- Test: `scripts/test_dimensio_acceptance.py`

- [ ] **Step 1: Add imports for CLI error capture**

Replace the current contextlib import with:

```python
from contextlib import contextmanager, redirect_stderr
```

Add:

```python
from io import StringIO
```

- [ ] **Step 2: Add exact payload and CLI tests before the existing lifecycle test class**

Insert this class immediately before `DimensioAcceptanceTest`:

```python
class DimensioModePayloadTest(unittest.TestCase):
    def test_cli_defaults_to_image_and_accepts_all_modes(self) -> None:
        self.assertEqual(acceptance._parse_args([]).mode, "image")
        for mode in ("text", "image", "multimodal"):
            with self.subTest(mode=mode):
                self.assertEqual(
                    acceptance._parse_args(["--mode", mode]).mode, mode
                )

        with patch.object(
            acceptance,
            "run_acceptance",
            return_value=(0, Path(".")),
        ) as run_acceptance:
            self.assertEqual(acceptance.main([]), 0)
            run_acceptance.assert_called_once_with(mode="image")

    def test_cli_rejects_unknown_mode_with_exit_code_2(self) -> None:
        with redirect_stderr(StringIO()), self.assertRaises(
            SystemExit
        ) as raised:
            acceptance._parse_args(["--mode", "unknown"])
        self.assertEqual(raised.exception.code, 2)

    def test_build_payload_returns_exact_mode_shapes(self) -> None:
        text_payload = acceptance.build_payload("text")
        image_payload = acceptance.build_payload("image")
        multimodal_payload = acceptance.build_payload("multimodal")

        for payload in (text_payload, image_payload, multimodal_payload):
            self.assertEqual(
                {
                    "model": payload["model"],
                    "ratio": payload["ratio"],
                    "resolution": payload["resolution"],
                    "duration": payload["duration"],
                },
                {
                    "model": "jimeng-video-seedance-2.0-fast-vip",
                    "ratio": "16:9",
                    "resolution": "720p",
                    "duration": 4,
                },
            )

        self.assertEqual(
            [item["type"] for item in text_payload["content"]], ["text"]
        )
        self.assertEqual(
            [
                (item["type"], item.get("role"))
                for item in image_payload["content"]
            ],
            [
                ("image_url", "first_frame"),
                ("image_url", "last_frame"),
                ("text", None),
            ],
        )
        self.assertEqual(
            [
                (item["type"], item.get("role"))
                for item in multimodal_payload["content"]
            ],
            [
                ("image_url", "reference_image"),
                ("image_url", "reference_image"),
                ("image_url", "reference_image"),
                ("image_url", "reference_image"),
                ("video_url", "reference_video"),
                ("audio_url", "reference_audio"),
                ("text", None),
            ],
        )
        self.assertEqual(
            multimodal_payload["content"][4]["video_url"]["url"],
            (
                "https://media.githubusercontent.com/media/adilentiq/"
                "test-images/02f54833d4d08fc33d2090eaeda9f0d2dbf1c7b0/"
                "video/duration/v1_4s.mp4"
            ),
        )

    def test_build_payload_returns_independent_objects(self) -> None:
        first = acceptance.build_payload("image")
        second = acceptance.build_payload("image")
        first["content"][0]["role"] = "changed"
        self.assertEqual(second["content"][0]["role"], "first_frame")
```

In the existing successful lifecycle test, replace:

```python
            self.assertEqual(
                state.gateway_requests[0]["json"], acceptance.FIXED_PAYLOAD
            )
```

with:

```python
            self.assertEqual(
                state.gateway_requests[0]["json"],
                acceptance.build_payload("image"),
            )
```

- [ ] **Step 3: Run the focused test class and verify RED**

Run from the repository root:

```bash
python -m unittest scripts.test_dimensio_acceptance.DimensioModePayloadTest -v
```

Expected: 4 tests fail because `_parse_args` and `build_payload` do not exist and `main` does not accept an argument list. The failure must be caused by missing mode behavior, not a syntax or import error.

### Task 2: Add the mode registry, payload selection, and CLI

**Files:**
- Modify: `scripts/dimensio_acceptance.py:4-36`
- Modify: `scripts/dimensio_acceptance.py:227-392`
- Test: `scripts/test_dimensio_acceptance.py`

- [ ] **Step 1: Import argparse and deepcopy**

Add these imports:

```python
import argparse
from copy import deepcopy
```

- [ ] **Step 2: Replace the single fixed payload with the exact mode registry**

Replace `FIXED_PAYLOAD` with:

```python
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
    "audio_1": "https://www.w3schools.com/html/horse.mp3",
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

```

- [ ] **Step 3: Add payload construction and CLI parsing after AcceptanceError**

```python
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
```

- [ ] **Step 4: Thread the selected mode and payload through run_acceptance**

Change the signature to:

```python
def run_acceptance(
    *,
    mode: str = DEFAULT_MODE,
    base_url: str = BASE_URL,
    api_key: str = API_KEY,
    output_root: Path = OUTPUT_ROOT,
    poll_interval_seconds: float = POLL_INTERVAL_SECONDS,
    max_wait_seconds: float = MAX_WAIT_SECONDS,
) -> tuple[int, Path]:
```

Initialize these report fields instead of storing `FIXED_PAYLOAD`:

```python
        "mode": mode,
        "request": {
            "base_url": base_url,
            "payload": None,
        },
```

Immediately after configuration validation, build and store the selected payload:

```python
        payload = build_payload(mode)
        report["request"]["payload"] = payload
```

Submit `payload` rather than `FIXED_PAYLOAD`:

```python
        submit_response = _request_json(
            "POST", submit_url, normalized_api_key, payload
        )
```

- [ ] **Step 5: Replace main with the argparse entry point**

```python
def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    exit_code, _ = run_acceptance(mode=args.mode)
    return exit_code
```

- [ ] **Step 6: Run the focused class and full existing suite**

```bash
python -m unittest scripts.test_dimensio_acceptance.DimensioModePayloadTest -v
python -m unittest scripts.test_dimensio_acceptance -v
```

Expected: the focused 4 tests pass and the full suite reports 8 tests with `OK`. The existing successful lifecycle now submits the default image payload returned by `build_payload("image")`; no media preflight exists yet.

- [ ] **Step 7: Commit the mode selection behavior**

```bash
git add scripts/dimensio_acceptance.py scripts/test_dimensio_acceptance.py
git commit -m "feat(dimensio): add acceptance test modes"
```

### Task 3: Add failing anonymous media-preflight and mode-lifecycle tests

**Files:**
- Modify: `scripts/test_dimensio_acceptance.py:16-96`
- Modify: `scripts/test_dimensio_acceptance.py:98-273`
- Test: `scripts/test_dimensio_acceptance.py`

- [ ] **Step 1: Extend MockState with local asset responses**

Add:

```python
    asset_responses: dict[str, tuple[int, str, bytes]] = field(
        default_factory=dict
    )
    asset_requests: list[dict] = field(default_factory=list)
```

- [ ] **Step 2: Teach MockHandler to serve HEAD and ranged GET asset probes**

Add these methods before `do_POST`:

```python
    def _send_asset(
        self,
        status: int,
        content_type: str,
        body: bytes,
        *,
        include_body: bool,
    ) -> None:
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if include_body:
            self.wfile.write(body)

    def do_HEAD(self) -> None:
        state = self.server.state
        path = urlsplit(self.path).path
        state.asset_requests.append(
            {
                "method": "HEAD",
                "path": path,
                "headers": self._headers(),
            }
        )
        status, content_type, body = state.asset_responses[path]
        self._send_asset(
            status, content_type, body, include_body=False
        )
```

At the start of `do_GET`, after computing `path` and `request` but before the result-video branch, add:

```python
        if path in state.asset_responses:
            state.asset_requests.append(request)
            status, content_type, body = state.asset_responses[path]
            self._send_asset(
                status, content_type, body, include_body=True
            )
            return
```

- [ ] **Step 3: Add successful anonymous preflight coverage**

Insert this class after `DimensioModePayloadTest`:

```python
class DimensioAssetPreflightTest(unittest.TestCase):
    def test_preflight_validates_image_audio_and_mp4_without_key(self) -> None:
        mp4_header = b"\x00\x00\x00\x18ftypisom"
        state = MockState(
            asset_responses={
                "/assets/image.jpg": (200, "image/jpeg", b"jpeg"),
                "/assets/audio.mp3": (200, "audio/mpeg", b"mp3"),
                "/assets/video.mp4": (
                    200,
                    "application/octet-stream",
                    mp4_header,
                ),
            }
        )
        with serve(state) as base_url:
            payload = {
                "content": [
                    {
                        "type": "image_url",
                        "role": "reference_image",
                        "image_url": {
                            "url": f"{base_url}/assets/image.jpg"
                        },
                    },
                    {
                        "type": "audio_url",
                        "role": "reference_audio",
                        "audio_url": {
                            "url": f"{base_url}/assets/audio.mp3"
                        },
                    },
                    {
                        "type": "video_url",
                        "role": "reference_video",
                        "video_url": {
                            "url": f"{base_url}/assets/video.mp4"
                        },
                    },
                ]
            }
            checks = acceptance._preflight_assets(payload)

        self.assertEqual(
            [check["type"] for check in checks],
            ["image", "audio", "video"],
        )
        self.assertTrue(all(check["ok"] for check in checks))
        self.assertEqual(
            [request["method"] for request in state.asset_requests],
            ["HEAD", "HEAD", "GET"],
        )
        for request in state.asset_requests:
            self.assertNotIn("authorization", request["headers"])

    def test_preflight_rejects_http_type_and_mp4_errors(self) -> None:
        cases = [
            (
                "http",
                "/assets/bad.jpg",
                {
                    "type": "image_url",
                    "image_url": {"url": ""},
                },
                (503, "image/jpeg", b""),
            ),
            (
                "image_type",
                "/assets/bad.jpg",
                {
                    "type": "image_url",
                    "image_url": {"url": ""},
                },
                (200, "text/plain", b"not-image"),
            ),
            (
                "audio_type",
                "/assets/bad.mp3",
                {
                    "type": "audio_url",
                    "audio_url": {"url": ""},
                },
                (200, "text/plain", b"not-audio"),
            ),
            (
                "mp4_header",
                "/assets/bad.mp4",
                {
                    "type": "video_url",
                    "video_url": {"url": ""},
                },
                (200, "application/octet-stream", b"not-an-mp4!"),
            ),
        ]
        for label, path, item, response in cases:
            with self.subTest(label=label):
                state = MockState(asset_responses={path: response})
                with serve(state) as base_url:
                    item[item["type"]]["url"] = base_url + path
                    with self.assertRaises(
                        acceptance.AcceptanceError
                    ) as raised:
                        acceptance._preflight_assets(
                            {"content": [item]}
                        )
                self.assertEqual(raised.exception.category, "asset")

    def test_asset_failure_writes_report_before_gateway_submission(self) -> None:
        state = MockState(
            asset_responses={
                "/assets/missing.jpg": (404, "image/jpeg", b"")
            }
        )
        with TemporaryDirectory() as temp_dir, serve(state) as base_url:
            payload = {
                "model": "jimeng-video-seedance-2.0-fast-vip",
                "content": [
                    {
                        "type": "image_url",
                        "role": "first_frame",
                        "image_url": {
                            "url": f"{base_url}/assets/missing.jpg"
                        },
                    },
                    {"type": "text", "text": "test"},
                ],
                "ratio": "16:9",
                "resolution": "720p",
                "duration": 4,
            }
            with patch.object(
                acceptance, "build_payload", return_value=payload
            ):
                exit_code, run_dir = acceptance.run_acceptance(
                    mode="image",
                    base_url=base_url,
                    api_key="test-secret-1234",
                    output_root=Path(temp_dir),
                    poll_interval_seconds=0,
                    max_wait_seconds=5,
                )

            report = json.loads(
                (run_dir / "report.json").read_text(encoding="utf-8")
            )
            self.assertEqual(exit_code, 1)
            self.assertEqual(report["error"]["category"], "asset")
            self.assertEqual(state.gateway_requests, [])
```

- [ ] **Step 4: Add a failing exact-payload lifecycle matrix**

Add this method to `DimensioAcceptanceTest`:

```python
    def test_each_mode_submits_its_exact_payload(self) -> None:
        for mode in acceptance.ACCEPTANCE_MODES:
            with self.subTest(mode=mode), TemporaryDirectory() as temp_dir:
                state = MockState()
                with serve(state) as base_url, patch.object(
                    acceptance, "_preflight_assets", return_value=[]
                ):
                    state.polls = [
                        (
                            200,
                            {
                                "id": "public-task-1",
                                "status": "succeeded",
                                "content": {
                                    "video_url": f"{base_url}/video.mp4"
                                },
                            },
                        )
                    ]
                    exit_code, run_dir = acceptance.run_acceptance(
                        mode=mode,
                        base_url=base_url,
                        api_key="test-secret-1234",
                        output_root=Path(temp_dir),
                        poll_interval_seconds=0,
                        max_wait_seconds=5,
                    )

                self.assertEqual(exit_code, 0)
                self.assertEqual(
                    state.gateway_requests[0]["json"],
                    acceptance.build_payload(mode),
                )
                report, _ = self.read_report(run_dir)
                self.assertEqual(report["mode"], mode)
                self.assertEqual(
                    report["request"]["payload"],
                    acceptance.build_payload(mode),
                )
```

- [ ] **Step 5: Isolate existing lifecycle tests from public assets**

In `test_successful_lifecycle_downloads_video_and_redacts_key`, add a preflight patch to the existing context:

```python
        with TemporaryDirectory() as temp_dir, serve(
            state
        ) as base_url, patch.object(
            acceptance, "_preflight_assets", return_value=[]
        ):
```

Add `mode="text"` to the `run_acceptance` calls in:

```python
    test_failed_task_reports_error_and_does_not_download
    test_non_2xx_ark_and_top_level_errors_are_preserved
    test_invalid_configuration_fails_before_network_and_writes_report
```

Each affected call must begin:

```python
            exit_code, run_dir = acceptance.run_acceptance(
                mode="text",
```

For the looped non-2xx and configuration cases, repeat the same explicit `mode="text"` argument in their existing calls.

- [ ] **Step 6: Run the full test module and verify RED**

```bash
python -m unittest scripts.test_dimensio_acceptance -v
```

Expected: the new preflight tests and every test patching `_preflight_assets` fail because that function does not exist. Existing mode payload tests remain green.

### Task 4: Implement anonymous asset preflight and report integration

**Files:**
- Modify: `scripts/dimensio_acceptance.py:36-43`
- Modify: `scripts/dimensio_acceptance.py:83-173`
- Modify: `scripts/dimensio_acceptance.py:227-385`
- Test: `scripts/test_dimensio_acceptance.py`

- [ ] **Step 1: Add the preflight timeout**

Add next to the existing timeouts:

```python
ASSET_TIMEOUT_SECONDS = 20
```

- [ ] **Step 2: Add asset extraction and anonymous preflight**

Insert after `_validate_config`:

```python
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
```

The only headers in these requests are `Accept`, `User-Agent`, and the video `Range` header. Do not pass `api_key` into either helper.

- [ ] **Step 3: Add report fields and call preflight before submission**

Add these initial report fields:

```python
        "assets": [],
        "asset_checks": [],
```

After `payload = build_payload(mode)` and before constructing or calling the submit request:

```python
        report["request"]["payload"] = payload
        report["assets"] = _extract_assets(payload)
        report["asset_checks"] = _preflight_assets(payload)
```

Configuration validation must remain before payload preflight. This preserves the contract that a placeholder API key fails without contacting public assets or the gateway.

- [ ] **Step 4: Run focused preflight tests and verify GREEN**

```bash
python -m unittest scripts.test_dimensio_acceptance.DimensioAssetPreflightTest -v
```

Expected: 3 tests pass. The local server sees HEAD, HEAD, GET in order and no asset request has an Authorization header.

- [ ] **Step 5: Run the complete Python suite**

```bash
python -m unittest scripts.test_dimensio_acceptance -v
```

Expected: 12 tests pass with `OK`, including all three mode lifecycle subtests and the existing error/redaction cases.

- [ ] **Step 6: Commit preflight and lifecycle coverage**

```bash
git add scripts/dimensio_acceptance.py scripts/test_dimensio_acceptance.py
git commit -m "test(dimensio): validate multimodal acceptance assets"
```

### Task 5: Run deterministic verification and inspect repository scope

**Files:**
- Verify: `scripts/dimensio_acceptance.py`
- Verify: `scripts/test_dimensio_acceptance.py`

- [ ] **Step 1: Compile both Python modules**

```bash
python -m py_compile scripts/dimensio_acceptance.py scripts/test_dimensio_acceptance.py
```

Expected: exit code 0 with no output.

- [ ] **Step 2: Run the full Python suite without bytecode artifacts**

From PowerShell:

```powershell
$env:PYTHONDONTWRITEBYTECODE = '1'
python -m unittest scripts.test_dimensio_acceptance -v
```

Expected: 12 tests pass with no network request outside the local `ThreadingHTTPServer`.

- [ ] **Step 3: Re-run the backend Dimensio role and translation contract**

```bash
go test ./relay/channel/task/dimensio -run 'TestArkToDimensioMultimodal|TestDeriveFunctionModeMatrix|TestArkToDimensioRejectsMediaLimits|TestDimensioSeedance20ProtocolE2E' -count=1
```

Expected: package passes. This proves the client roles still map to `first_last_frames` and `omni_reference` within Dimensio's media limits.

- [ ] **Step 4: Verify the placeholder CLI stops before asset or gateway traffic**

Leave `API_KEY = "replace-with-new-api-key"` and run:

```bash
python scripts/dimensio_acceptance.py
```

Expected: exit code 1, a report with `mode=image` and `error.category=config`, no `asset_checks`, and no `video.mp4`. Remove this generated placeholder run after inspection and do not stage it.

- [ ] **Step 5: Inspect Git scope**

```bash
git status --short
git diff --check
git log -3 --oneline
```

Expected: the two implementation commits are present; no generated report, downloaded video, API Key, or unrelated user file is staged or committed.

### Task 6: Run all three real modes and cross-check logs

**Files:**
- Produce locally: `output/dimensio-client-acceptance/<run-id>/video.mp4`
- Produce locally: `output/dimensio-client-acceptance/<run-id>/report.json`
- Inspect: `http://127.0.0.1:3000/usage-logs/task`
- Inspect: `http://127.0.0.1:3000/usage-logs/common`

- [ ] **Step 1: Confirm the expected cost before running**

Three successful Fast VIP 720p, 4-second tasks consume `3 * 192 = 576` Dimensio points. The written design already authorizes these three real acceptance calls. Do not add retries that submit replacement tasks automatically.

The multimodal request must use the commit-pinned `v1_4s.mp4` asset from the mode registry. Its MP4 `mvhd` duration was measured at 4.967 seconds, satisfying the requested 4-5 second reference-video range.

- [ ] **Step 2: Run image, multimodal, and text sequentially without writing the API key**

Run this PowerShell command from the repository root:

```powershell
@'
import getpass
from scripts.dimensio_acceptance import run_acceptance

api_key = getpass.getpass("API key: ")
failures = []
for mode in ("image", "multimodal", "text"):
    exit_code, run_dir = run_acceptance(
        mode=mode,
        base_url="http://127.0.0.1:3000",
        api_key=api_key,
    )
    print(f"{mode}: exit={exit_code} artifacts={run_dir.resolve()}")
    if exit_code != 0:
        failures.append(mode)
raise SystemExit(1 if failures else 0)
'@ | python -
```

Expected: one hidden API-key prompt, three sequential tasks, three exit codes of 0, and three distinct artifact directories. The first call is exactly the same mode selected by the no-argument CLI default.

- [ ] **Step 3: Validate every real artifact**

For each printed directory verify:

- `report.json` has the selected `mode`, `result=passed`, terminal `succeeded`, expected payload shape, completed asset checks, public task ID, and `api_key_hint` only.
- `video.mp4` is non-empty and bytes 4-7 equal `ftyp`.
- The report contains no substring starting with `sk-`.
- `text` has zero assets, `image` has two assets, and `multimodal` has six assets including an MP4 check with `mp4_ftyp=true`.

- [ ] **Step 4: Cross-check task and usage logs**

In the authenticated local console, match each public task ID from the reports:

- Task log: channel `#5`, channel type `59`, user `admin`, status success, progress 100%, and full task duration. If the UI still shows a generic “图生视频” label, record it as a display limitation instead of treating it as a routing failure.
- Usage log: model `jimeng-video-seedance-2.0-fast-vip`, non-stream request, fee `¥1.92` per task, group `default`, and matching submission time.
- Dimensio upstream log: 192 points per task and the intended `first_last_frames` or `omni_reference` mode.

Record any display-only discrepancy separately from routing, task status, and billing correctness. Do not change the backend or frontend under this client-script plan.
