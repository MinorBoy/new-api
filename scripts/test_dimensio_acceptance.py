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
