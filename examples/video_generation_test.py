#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
secure-skill 渠道视频生成端到端测试脚本

测试流程（基于 https://token.secure-skill.com/docs#video-generation）：
    1. POST /api/generate-video                        # multipart 提交生成任务，拿到 task_id
    2. GET  /api/task/{task_id}                         # 轮询任务状态，直到 completed / failed
    3. GET  /api/task/{task_id}/video-link?refresh=1    # 拿到视频直链
    4. GET  /api/video/{task_id}?license_key=...        # （可选）下载视频到本地

全程打印各阶段耗时与累计生成耗时。

认证：X-License-Key 请求头（注意不是 Authorization: Bearer）。
提交任务必须用 multipart/form-data，本脚本会强制走 multipart。

使用：
    export SECURE_SKILL_KEY="你的 X-License-Key"
    export SECURE_SKILL_BASE_URL="https://token.secure-skill.com"  # 可选，默认同左

    python video_generation_test.py                       # 使用默认 prompt
    python video_generation_test.py --prompt "一个女孩在跳舞" --duration 5
    python video_generation_test.py --download out.mp4    # 完成后下载视频

依赖：requests（pip install requests）
"""

import argparse
import os
import sys
import time
from pathlib import Path

try:
    import requests
    from urllib3.filepost import encode_multipart_formdata
except ImportError:  # pragma: no cover
    sys.stderr.write("缺少 requests 库，请先执行：pip install requests\n")
    sys.exit(1)


DEFAULT_BASE_URL = "https://token.secure-skill.com"
DEFAULT_PROMPT = "一只柴犬在草地上奔跑，电影感镜头"
DEFAULT_DURATION = 4
DEFAULT_RATIO = "9:16"
DEFAULT_RESOLUTION = "720p"
DEFAULT_MODEL = "sd-2.0-pro"
POLL_INTERVAL = 5            # 每次轮询间隔（秒）
POLL_TIMEOUT = 600           # 轮询总超时（秒）
TERMINAL_FAILED = "failed"


# ---------- 通用工具 ----------

def _env(name: str, default=None, required: bool = False) -> str:
    val = os.environ.get(name, default)
    if required and not val:
        sys.stderr.write(f"环境变量 {name} 未设置\n")
        sys.exit(1)
    return val


def _h(status_code: int) -> str:
    """短小的状态码标记，便于日志阅读。"""
    if 200 <= status_code < 300:
        return "OK"
    if status_code in (401, 403):
        return "AUTH_FAIL"
    if status_code == 404:
        return "NOT_FOUND"
    if status_code == 429:
        return "RATE_LIMIT"
    if status_code >= 500:
        return "SERVER_ERR"
    return f"HTTP_{status_code}"


def _now() -> float:
    return time.monotonic()


def _fmt_seconds(seconds: float) -> str:
    if seconds < 60:
        return f"{seconds:.2f}s"
    minutes, rest = divmod(int(seconds), 60)
    return f"{minutes}m{rest:02d}s"


# ---------- 各阶段实现 ----------

def _force_multipart(fields: dict, files: list | None = None
                     ) -> tuple[bytes, str]:
    """
    手动用 urllib3 编码 multipart/form-data。

    为什么不用 requests 的 files= 参数：requests 在 files 为空列表/None 时
    会退化成 application/x-www-form-urlencoded，而本服务端强制要求 multipart。
    这里直接调用 urllib3（requests 自带依赖，无需额外安装）的编码器，保证
    无论是否有上传文件都会发送 multipart/form-data。
    """
    fields = {k: (None, str(v)) for k, v in fields.items()}
    if files:
        fields.update({name: (fname, fobj, mime)
                       for name, (fname, fobj, mime) in files})
    body, content_type = encode_multipart_formdata(fields)
    return body, content_type


def submit_task(session: requests.Session, base_url: str, headers: dict,
                prompt: str, duration: int, ratio: str, resolution: str,
                model: str, protect_stripe: bool,
                reference_image: Path | None) -> tuple[str, dict, float]:
    """
    提交视频生成任务。

    返回 (task_id, response_json, elapsed_seconds)。
    """
    url = f"{base_url}/api/generate-video"

    # multipart/form-data —— 文档规定字段走表单。
    form = {
        "prompt": prompt,
        "duration": str(duration),
        "ratio": ratio,
        "resolution": resolution,
        "model": model,
        "protect_stripe": "true" if protect_stripe else "false",
    }

    # 收集需要上传的文件（图生视频）。name 字段固定为 "files"。
    files = []
    if reference_image is not None:
        files.append(("files", (reference_image.name,
                                open(reference_image, "rb"),
                                "application/octet-stream")))

    # 手动编码为 multipart，避免 requests 在无文件时退化为 urlencoded。
    body, content_type = _force_multipart(form, files)

    # 保留 Accept 等已有头，覆盖 Content-Type；不要让 requests 自动加
    # application/x-www-form-urlencoded。
    send_headers = dict(headers)
    send_headers["Content-Type"] = content_type

    start = _now()
    try:
        resp = session.post(url, headers=send_headers, data=body, timeout=60)
    finally:
        for _, (_name, fobj, _mime) in files:
            fobj.close()

    elapsed = _now() - start
    print(f"[1/4] 提交任务  POST /api/generate-video  "
          f"-> {_h(resp.status_code)} ({_fmt_seconds(elapsed)})")

    if resp.status_code >= 400:
        sys.stderr.write(f"     提交失败：{resp.status_code} {resp.text[:500]}\n")
        sys.exit(2)

    try:
        data = resp.json()
    except ValueError:
        sys.stderr.write(f"     返回不是 JSON：{resp.text[:500]}\n")
        sys.exit(2)

    # 兼容 {"id": "..."} / {"task_id": "..."} / {"data": {"id": "..."}} 三种风格
    task_id = (data.get("id")
               or data.get("task_id")
               or (data.get("data", {}) or {}).get("id"))
    if not task_id:
        sys.stderr.write(f"     返回中找不到 task id：{data}\n")
        sys.exit(2)

    print(f"     task_id = {task_id}")
    return task_id, data, elapsed


def poll_status(session: requests.Session, base_url: str, headers: dict,
                task_id: str) -> tuple[dict, float]:
    """
    轮询任务状态，直到状态为 completed 或 failed，或达到超时。

    返回 (最终状态 JSON, 累计轮询耗时)。
    """
    url = f"{base_url}/api/task/{task_id}"
    deadline = _now() + POLL_TIMEOUT
    start = _now()

    last_status = None
    while _now() < deadline:
        resp = session.get(url, headers=headers, timeout=30)
        if resp.status_code == 404:
            sys.stderr.write(f"     任务不存在：{task_id}\n")
            sys.exit(2)
        if resp.status_code >= 400:
            sys.stderr.write(f"     查询状态失败：{resp.status_code} {resp.text[:300]}\n")
            time.sleep(POLL_INTERVAL)
            continue

        data = resp.json()
        status = (data.get("status")
                  or (data.get("data", {}) or {}).get("status")
                  or "unknown")
        # 进度打印（仅状态变化时输出一行，避免噪声）
        if status != last_status:
            elapsed = _now() - start
            print(f"[2/4] 状态变更  status={status}  "
                  f"(累计 {_fmt_seconds(elapsed)})")
            last_status = status

        if status == "completed":
            return data, _now() - start
        if status == TERMINAL_FAILED:
            sys.stderr.write(f"     任务失败：{data}\n")
            sys.exit(3)

        time.sleep(POLL_INTERVAL)

    sys.stderr.write(f"     轮询超时（>{POLL_TIMEOUT}s），最后状态：{last_status}\n")
    sys.exit(4)


def fetch_video_link(session: requests.Session, base_url: str, headers: dict,
                     task_id: str) -> tuple[str, dict, float]:
    """
    获取视频直链。带 refresh=1 强制刷新临时链接。
    """
    url = f"{base_url}/api/task/{task_id}/video-link"
    start = _now()
    resp = session.get(url, headers=headers,
                       params={"refresh": 1}, timeout=30)
    elapsed = _now() - start
    print(f"[3/4] 获取直链  GET /api/task/{task_id}/video-link  "
          f"-> {_h(resp.status_code)} ({_fmt_seconds(elapsed)})")

    if resp.status_code >= 400:
        sys.stderr.write(f"     获取直链失败：{resp.status_code} {resp.text[:500]}\n")
        sys.exit(2)

    data = resp.json()
    link = (data.get("video_url")
            or data.get("url")
            or data.get("link")
            or (data.get("data", {}) or {}).get("video_url")
            or (data.get("data", {}) or {}).get("url"))
    if not link:
        sys.stderr.write(f"     返回中找不到 video_url：{data}\n")
        sys.exit(2)

    print(f"     video_url = {link}")
    return link, data, elapsed


def download_video(session: requests.Session, base_url: str, license_key: str,
                   task_id: str, dest: Path) -> float:
    """
    下载视频到本地。
    """
    url = f"{base_url}/api/video/{task_id}"
    start = _now()
    resp = session.get(url, params={"license_key": license_key},
                       stream=True, timeout=120)
    elapsed = _now() - start
    if resp.status_code >= 400:
        sys.stderr.write(f"     下载失败：{resp.status_code} {resp.text[:300]}\n")
        sys.exit(2)

    written = 0
    with open(dest, "wb") as f:
        for chunk in resp.iter_content(chunk_size=64 * 1024):
            if chunk:
                f.write(chunk)
                written += len(chunk)

    size_str = (f"{written / 1024 / 1024:.2f}MB"
                if written else f"{resp.headers.get('Content-Length', '?')}B")
    print(f"[4/4] 下载视频  -> {dest} ({size_str}, {_fmt_seconds(elapsed)})")
    return elapsed


# ---------- 主流程 ----------

def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="secure-skill 渠道视频生成端到端测试")
    p.add_argument("--base-url",
                   default=_env("SECURE_SKILL_BASE_URL", DEFAULT_BASE_URL))
    p.add_argument("--key",
                   default=_env("SECURE_SKILL_KEY", required=True),
                   help="X-License-Key（也可用环境变量 SECURE_SKILL_KEY）")
    p.add_argument("--prompt", default=DEFAULT_PROMPT)
    p.add_argument("--duration", type=int, default=DEFAULT_DURATION,
                   help="视频时长（秒），通常 4 / 5 / 10")
    p.add_argument("--ratio", default=DEFAULT_RATIO,
                   choices=["16:9", "9:16", "1:1", "4:3", "3:4"])
    p.add_argument("--resolution", default=DEFAULT_RESOLUTION,
                   help="分辨率，例如 720p")
    p.add_argument("--model", default=DEFAULT_MODEL,
                   help="模型名，例如 sd-2.0-pro")
    p.add_argument("--protect-stripe", action="store_true",
                   help="开启防伪水印（默认关闭）")
    p.add_argument("--reference-image", type=Path, default=None,
                   help="参考图（可选），图生视频时使用")
    p.add_argument("--download", type=Path, default=None,
                   help="完成后下载视频到此路径（可选）")
    p.add_argument("--poll-interval", type=int, default=POLL_INTERVAL)
    p.add_argument("--poll-timeout", type=int, default=POLL_TIMEOUT)
    return p.parse_args()


def main() -> None:
    args = parse_args()

    global POLL_INTERVAL, POLL_TIMEOUT
    POLL_INTERVAL = args.poll_interval
    POLL_TIMEOUT = args.poll_timeout

    base_url = args.base_url.rstrip("/")
    headers = {
        "X-License-Key": args.key,
        "Accept": "application/json",
    }

    session = requests.Session()

    print("=" * 64)
    print(f"base_url       = {base_url}")
    print(f"prompt         = {args.prompt!r}")
    print(f"duration       = {args.duration}")
    print(f"ratio          = {args.ratio}")
    print(f"resolution     = {args.resolution}")
    print(f"model          = {args.model}")
    print(f"protect_stripe = {args.protect_stripe}")
    print(f"reference      = {args.reference_image or '(无)'}")
    print("=" * 64)

    overall_start = _now()

    # 1. 提交
    task_id, submit_resp, submit_t = submit_task(
        session, base_url, headers,
        prompt=args.prompt, duration=args.duration, ratio=args.ratio,
        resolution=args.resolution, model=args.model,
        protect_stripe=args.protect_stripe,
        reference_image=args.reference_image,
    )

    # 2. 轮询
    final_status, poll_t = poll_status(session, base_url, headers, task_id)

    # 3. 直链
    video_url, link_resp, link_t = fetch_video_link(
        session, base_url, headers, task_id)

    # 4. （可选）下载
    download_t = 0.0
    if args.download is not None:
        download_t = download_video(
            session, base_url, args.key, task_id, args.download)

    overall = _now() - overall_start

    print("=" * 64)
    print("测试结果汇总：")
    print(f"  task_id            = {task_id}")
    print(f"  final_status       = {final_status.get('status')}")
    print(f"  video_url          = {video_url}")
    print(f"  提交耗时            = {_fmt_seconds(submit_t)}")
    print(f"  生成耗时（轮询阶段） = {_fmt_seconds(poll_t)}")
    print(f"  获取直链耗时         = {_fmt_seconds(link_t)}")
    if args.download is not None:
        print(f"  下载耗时            = {_fmt_seconds(download_t)}")
    print(f"  端到端总耗时         = {_fmt_seconds(overall)}")
    print("=" * 64)


if __name__ == "__main__":
    main()
