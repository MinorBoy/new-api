import importlib.util
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("fetch_dimensio_models.py")
SPEC = importlib.util.spec_from_file_location("fetch_dimensio_models", SCRIPT_PATH)
if SPEC is None or SPEC.loader is None:
    raise ImportError(f"unable to load {SCRIPT_PATH}")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class _ResponseHandler(BaseHTTPRequestHandler):
    response_status = 200
    response_body = b"{}"

    def do_GET(self):  # noqa: N802
        self.send_response(self.response_status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(self.response_body)

    def log_message(self, *_args):
        return


class DimensioModelsTest(unittest.TestCase):
    def test_parse_models_payload_keeps_supported_fields_and_order(self):
        payload = {
            "data": [
                {
                    "id": " model-b ",
                    "object": "model",
                    "display_name": "B",
                    "media_type": "video",
                    "pricing": {"kind": "per_second"},
                },
                {"id": "model-a", "provider": "router"},
            ]
        }

        models = MODULE.parse_models_payload(payload)

        self.assertEqual([model["id"] for model in models], ["model-b", "model-a"])
        self.assertEqual(models[0]["pricing"], {"kind": "per_second"})
        self.assertEqual(models[1]["provider"], "router")

    def test_parse_models_payload_rejects_duplicate_ids(self):
        with self.assertRaisesRegex(ValueError, "duplicate model id"):
            MODULE.parse_models_payload({"data": [{"id": "same"}, {"id": "same"}]})

    def test_parse_models_payload_rejects_missing_or_empty_data(self):
        for payload in ({}, {"data": []}, {"data": [{"id": "  "}]}):
            with self.subTest(payload=payload):
                with self.assertRaises(ValueError):
                    MODULE.parse_models_payload(payload)

    def test_parse_models_payload_rejects_non_object_model(self):
        with self.assertRaisesRegex(ValueError, "model entry"):
            MODULE.parse_models_payload({"data": ["model-a"]})

    def test_fetch_models_rejects_non_success_response(self):
        _ResponseHandler.response_status = 502
        _ResponseHandler.response_body = b'{"data":[{"id":"must-not-be-used"}]}'
        server = ThreadingHTTPServer(("127.0.0.1", 0), _ResponseHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(thread.join, 2)
        self.addCleanup(server.shutdown)

        with self.assertRaisesRegex(RuntimeError, "status 502"):
            MODULE.fetch_models(f"http://127.0.0.1:{server.server_port}", timeout=2)

    def test_build_snapshot_is_json_serializable(self):
        models = [{"id": "model-a", "object": "model"}]

        snapshot = MODULE.build_snapshot(
            models,
            source_url="https://jimeng.dimensio.cn/v1/models",
            fetched_at="2026-08-23T00:00:00Z",
        )

        self.assertEqual(snapshot["model_count"], 1)
        self.assertEqual(snapshot["source_url"], "https://jimeng.dimensio.cn/v1/models")
        self.assertEqual(json.loads(json.dumps(snapshot))["models"], models)


if __name__ == "__main__":
    unittest.main()
