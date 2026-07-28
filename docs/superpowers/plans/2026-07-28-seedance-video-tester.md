# Seedance 视频供应商测试台 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone HTML workbench for testing Volcengine Ark Seedance video generation, including multimodal URL inputs, supplier profiles, automatic polling, and media/JSON inspection.

**Architecture:** Keep all markup, CSS, and browser-side JavaScript in one file under `examples/`. The page owns a small state model, serializes the native Ark request body, calls the create/get task endpoints with `fetch`, and renders status/output without a framework or build step.

**Tech Stack:** Semantic HTML, CSS custom properties, vanilla JavaScript, Lucide icon CDN, localStorage/session state, Fetch API, Playwright CLI for verification.

---

### Task 1: Add the standalone page shell and visual system

**Files:**
- Create: `docs/tools/ark-seedance-video-tester.html`

- [ ] Add semantic document structure with header, supplier panel, request workspace, result inspector, dialog, toast region, and responsive breakpoints.
- [ ] Add CSS custom properties for graphite surfaces, warm text, coral action, teal status, typography, spacing, stable control dimensions, focus states, and reduced-motion fallback.
- [ ] Load Lucide icons from CDN and initialize icons after DOM parsing; keep text labels for accessibility.

### Task 2: Implement supplier configuration and request editor

**Files:**
- Modify: `docs/tools/ark-seedance-video-tester.html`

- [ ] Add default Ark Beijing supplier (`https://ark.cn-beijing.volces.com`) and session-only provider management with optional localStorage persistence.
- [ ] Add model presets for `doubao-seedance-2-0-260128`, Fast, Mini, and 1.5 Pro with editable Model ID.
- [ ] Add prompt, resolution, ratio, duration, audio, watermark, return-last-frame, expiry, and advanced callback fields.
- [ ] Add repeatable URL inputs with limits of 9 images, 3 videos, and 3 audio URLs; expose count badges and removal controls.

### Task 3: Add request serialization, validation, and API workflow

**Files:**
- Modify: `docs/tools/ark-seedance-video-tester.html`

- [ ] Serialize content blocks into native Ark JSON, omitting empty optional values and preserving reference roles.
- [ ] Validate URL format, reference counts, duration range, required prompt/media, and the audio-only restriction before submit.
- [ ] Implement create-task fetch with Bearer auth, parse task id from native response, append timeline event, and start a 5-second poll loop.
- [ ] Implement manual task lookup, stop polling, terminal status handling, video/last-frame preview, raw JSON rendering, and CORS-aware error messages.
- [ ] Generate and copy an equivalent cURL command from the current supplier and request state.

### Task 4: Verify real user-facing behavior

**Files:**
- Test: `docs/tools/ark-seedance-video-tester.html` via Playwright CLI

- [ ] Open the page through a local HTTP server and capture desktop/mobile screenshots in `output/playwright/`.
- [ ] Exercise supplier add/remove, URL add/remove/limit, model compatibility, request JSON preview, cURL copy, and task-id lookup with mocked fetch responses.
- [ ] Confirm no console errors, controls remain accessible by label, and the standalone file opens without a build step.
