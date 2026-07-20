import json
import threading
import unittest
from contextlib import contextmanager, redirect_stderr
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from io import StringIO
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
    asset_responses: dict[str, tuple[int, str, bytes]] = field(
        default_factory=dict
    )
    asset_requests: list[dict] = field(default_factory=list)


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
        if path in state.asset_responses:
            state.asset_requests.append(request)
            status, content_type, body = state.asset_responses[path]
            self._send_asset(
                status, content_type, body, include_body=True
            )
            return
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


class DimensioAcceptanceTest(unittest.TestCase):
    def read_report(self, run_dir: Path) -> tuple[dict, str]:
        report_text = (run_dir / "report.json").read_text(encoding="utf-8")
        return json.loads(report_text), report_text

    def test_successful_lifecycle_downloads_video_and_redacts_key(self) -> None:
        state = MockState()
        api_key = "test-secret-1234"
        with TemporaryDirectory() as temp_dir, serve(
            state
        ) as base_url, patch.object(
            acceptance, "_preflight_assets", return_value=[]
        ):
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
                state.gateway_requests[0]["json"],
                acceptance.build_payload("image"),
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
            self.assertEqual(
                report["request"]["payload"],
                acceptance.build_payload("image"),
            )
            self.assertEqual(
                Path(report["video_path"]).read_bytes(), state.video_body
            )
            self.assertNotIn(api_key, report_text)

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
                mode="text",
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
                        mode="text",
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
                        mode="text",
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
