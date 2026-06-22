#!/usr/bin/env python3
"""mp_server.py - GoTuber Phase 2 MediaPipe Python sidecar.

Spec: docs/PHASE2.md Section 4.1, 4.3, Phase 2.1.

Captures the local webcam, runs MediaPipe Face Landmarker (Tasks API)
per frame, and publishes detection JSON (yaw/pitch/roll, EAR left/right,
face_center_x/y) on ZeroMQ PUB port 5556. The reverse channel (ZeroMQ
SUB on port 5555) is wired but only logged in Phase 2.1; Go-side frame
publishing lands in Phase 2.2/2.5.

YAGNI: this is the Phase 2.1 minimum. VIDEO mode, blendshape parsing,
multi-face, frame-driven detection, and ack semantics are intentionally
out of scope and left for later phases.
"""

from __future__ import annotations

import argparse
import json
import logging
import math
import signal
import sys
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any, Final, Optional

# cv2 / mediapipe / numpy / zmq are lazy-imported inside main() and the
# geometry helpers so that `python tools/mp_server.py --help` works in
# any Python 3.9+ interpreter without the .venv-mp env being activated.

logger = logging.getLogger("mp_server")


# ---------------------------------------------------------------------------
# Constants (stdlib-friendly; no numpy at module scope)
# ---------------------------------------------------------------------------

MODEL_URL: Final[str] = (
    "https://storage.googleapis.com/mediapipe-models/"
    "face_landmarker/face_landmarker/float16/latest/face_landmarker.task"
)

# Landmark indices per docs/PHASE2.md Phase 2.1.
LANDMARK_NOSE_TIP: Final[int] = 1
LANDMARK_FOREHEAD: Final[int] = 10
LANDMARK_CHIN: Final[int] = 152

# EAR landmark indices per docs/PHASE2.md Phase 2.1.
# Right eye (subject's left, image's right): 159/145/33/133.
_LANDMARK_RIGHT_EYE: Final[dict[str, int]] = {
    "top": 159, "bottom": 145, "outer": 33, "inner": 133,
}
# Left eye (subject's right, image's left): 386/374/263/362.
_LANDMARK_LEFT_EYE: Final[dict[str, int]] = {
    "top": 386, "bottom": 374, "outer": 263, "inner": 362,
}

# Behaviour knobs.
_FACE_LOST_WARN_SEC: Final[float] = 5.0      # warn after no-face for this long
_FPS_DEBUG_INTERVAL: Final[int] = 30         # --debug: log fps every N frames
_SUB_RECV_TIMEOUT_MS: Final[int] = 200       # SUB poll interval for graceful exit
_CAMERA_READ_RETRY_SEC: Final[float] = 0.05  # sleep when cap.read() fails
_FRAME_MODEL_MIN_BYTES: Final[int] = 1_000_000  # real model is ~3-5 MB


# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------

def _setup_logger(debug: bool) -> logging.Logger:
    """Configure root logger; throttles mediapipe to WARNING. Spec: PHASE2.md 2.1."""
    level = logging.DEBUG if debug else logging.INFO
    logging.basicConfig(
        level=level,
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
        datefmt="%H:%M:%S",
    )
    logging.getLogger("mediapipe").setLevel(logging.WARNING)
    return logging.getLogger("mp_server")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def _parse_args(argv: Optional[list[str]] = None) -> argparse.Namespace:
    """Parse CLI args; defaults match PHASE2.md Section 4.3 ports."""
    parser = argparse.ArgumentParser(
        prog="mp_server",
        description=(
            "GoTuber Phase 2 MediaPipe Python sidecar. Captures the local "
            "webcam, runs Face Landmarker, publishes detection JSON over "
            "ZeroMQ PUB. See docs/PHASE2.md Section 4.1/4.3."
        ),
    )
    parser.add_argument(
        "--frame-port", type=int, default=5555,
        help="ZeroMQ SUB port for inbound frames (Phase 2.5+ Go-side camera).",
    )
    parser.add_argument(
        "--detection-port", type=int, default=5556,
        help="ZeroMQ PUB port for outbound detection JSON (GoTuber subscribes).",
    )
    parser.add_argument(
        "--camera-id", type=int, default=0,
        help="OpenCV camera index passed to cv2.VideoCapture().",
    )
    parser.add_argument(
        "--model-path", type=str, default="assets/models/face_landmarker.task",
        help="Path to face_landmarker.task (auto-downloaded if missing).",
    )
    parser.add_argument(
        "--no-auto-download", action="store_true",
        help="Disable auto-download of face_landmarker.task (debug only).",
    )
    parser.add_argument(
        "--debug", action="store_true",
        help="Enable verbose (DEBUG level) logging.",
    )
    return parser.parse_args(argv)


# ---------------------------------------------------------------------------
# Model loading
# ---------------------------------------------------------------------------

def _ensure_model(
    model_path: Path, allow_download: bool, log: logging.Logger,
) -> Path:
    """Return existing model_path or download from MODEL_URL. Spec: PHASE2.md 2.1.

    Raises FileNotFoundError if missing and downloads disabled; RuntimeError
    if the download fails or yields a suspiciously small payload.
    """
    if model_path.is_file():
        log.debug(
            "Using existing model at %s (%d bytes)",
            model_path, model_path.stat().st_size,
        )
        return model_path

    if not allow_download:
        log.error(
            "Model not found at %s and auto-download disabled "
            "(--no-auto-download).", model_path,
        )
        raise FileNotFoundError(f"face_landmarker.task missing at {model_path}")

    model_path.parent.mkdir(parents=True, exist_ok=True)
    log.info("Downloading face_landmarker.task from %s ...", MODEL_URL)
    try:
        with urllib.request.urlopen(MODEL_URL, timeout=60) as resp:  # noqa: S310
            data = resp.read()
    except (urllib.error.URLError, TimeoutError, OSError) as e:
        log.error("Failed to download face_landmarker.task: %s", e)
        raise RuntimeError(f"Model download failed: {e}") from e

    if len(data) < _FRAME_MODEL_MIN_BYTES:
        raise RuntimeError(
            f"Downloaded model is suspiciously small "
            f"({len(data)} bytes < {_FRAME_MODEL_MIN_BYTES}); aborting"
        )

    model_path.write_bytes(data)
    log.info("Downloaded %d bytes to %s", len(data), model_path)
    return model_path


def _create_landmarker(model_path: Path, log: logging.Logger) -> Any:
    """Build a MediaPipe FaceLandmarker in IMAGE mode. Spec: PHASE2.md 2.1.

    IMAGE mode is the simplest for capture-rate loop. Phase 2.5+ may switch
    to VIDEO mode for timestamp-aware smoothing once the Go-side clock owns
    frame pacing.
    """
    import mediapipe as mp  # lazy: keep --help stdlib-only

    base_options = mp.tasks.python.BaseOptions(model_asset_path=str(model_path))
    options = mp.tasks.vision.FaceLandmarkerOptions(
        base_options=base_options,
        running_mode=mp.tasks.vision.RunningMode.IMAGE,
        num_faces=1,
        output_face_blendshapes=False,
        output_facial_transformation_matrixes=False,
    )
    log.info(
        "Creating FaceLandmarker (IMAGE mode, num_faces=1) from %s",
        model_path,
    )
    return mp.tasks.vision.FaceLandmarker.create_from_options(options)


# ---------------------------------------------------------------------------
# Geometry helpers (pure functions on numpy arrays)
# ---------------------------------------------------------------------------

def _get_face_model_3d() -> Any:
    """Canonical 3D face model (mm) used by solvePnP. Spec: PHASE2.md 2.1.

    Camera frame convention (OpenCV): X right, Y down, Z forward. Nose tip
    is the origin; forehead 100mm above (Y-) and chin 120mm below (Y+),
    both 10mm behind the nose (Z-) to give solvePnP a non-degenerate depth
    lever for pitch estimation.
    """
    import numpy as np
    return np.array(
        [
            [0.0,    0.0,   0.0],   # 1: nose tip
            [0.0, -100.0, -10.0],   # 10: forehead
            [0.0,  120.0, -10.0],   # 152: chin
        ],
        dtype=np.float64,
    )


def _landmarks_to_pixels(normalized_landmarks: Any, width: int, height: int) -> Any:
    """Convert MediaPipe normalized landmarks (x, y, z) to a pixel-space Nx3 array.

    z is scaled by width to roughly match OpenCV's depth convention used by
    solvePnP; this is the heuristic used by the official MediaPipe samples.
    """
    import numpy as np
    out = np.empty((len(normalized_landmarks), 3), dtype=np.float64)
    for i, lm in enumerate(normalized_landmarks):
        out[i, 0] = lm.x * width
        out[i, 1] = lm.y * height
        out[i, 2] = lm.z * width
    return out


def _compute_head_pose(
    landmarks_px: Any, frame_shape: tuple[int, int, int],
) -> tuple[float, float, float]:
    """solvePnP-based head yaw/pitch/roll (degrees). Spec: PHASE2.md 2.1.

    Three reference landmarks (nose tip 1, forehead 10, chin 152) matched
    against the canonical 3D face model. Uses cv2.SOLVEPNP_SQPNP for wide
    convergence. roll is returned for Phase 2.5+ lean detection; the
    Phase 2.4 Go-side mapper currently uses only yaw/pitch.
    """
    import cv2
    import numpy as np

    h, w = frame_shape[:2]
    image_points = np.array(
        [
            landmarks_px[LANDMARK_NOSE_TIP, :2],
            landmarks_px[LANDMARK_FOREHEAD, :2],
            landmarks_px[LANDMARK_CHIN, :2],
        ],
        dtype=np.float64,
    )
    focal_length = float(max(w, h))
    camera_matrix = np.array(
        [
            [focal_length, 0.0, w / 2.0],
            [0.0, focal_length, h / 2.0],
            [0.0, 0.0, 1.0],
        ],
        dtype=np.float64,
    )
    dist_coeffs = np.zeros((4, 1), dtype=np.float64)

    ok, rvec, _tvec = cv2.solvePnP(
        _get_face_model_3d(),
        image_points,
        camera_matrix,
        dist_coeffs,
        flags=cv2.SOLVEPNP_SQPNP,
    )
    if not ok:
        logger.debug("solvePnP failed; returning zeros")
        return 0.0, 0.0, 0.0

    rot_mat, _ = cv2.Rodrigues(rvec)

    # Rotation matrix -> (pitch, yaw, roll) in radians, OpenCV camera frame
    # (X right, Y down, Z forward). Pitch about X, yaw about Y, roll about Z.
    sy = math.sqrt(rot_mat[0, 0] * rot_mat[0, 0] + rot_mat[1, 0] * rot_mat[1, 0])
    if sy >= 1e-6:
        pitch = math.atan2(rot_mat[2, 1], rot_mat[2, 2])
        yaw = math.atan2(-rot_mat[2, 0], sy)
        roll = math.atan2(rot_mat[1, 0], rot_mat[0, 0])
    else:
        pitch = math.atan2(-rot_mat[1, 2], rot_mat[1, 1])
        yaw = math.atan2(-rot_mat[2, 0], sy)
        roll = 0.0
    return float(np.degrees(yaw)), float(np.degrees(pitch)), float(np.degrees(roll))


def _compute_ear(landmarks_px: Any, eye: str) -> float:
    """Eye Aspect Ratio: vertical eyelid / horizontal canthus width.

    Uses MediaPipe indices from PHASE2.md 2.1 (right eye: 159/145/33/133,
    left eye: 386/374/263/362). Returns 0.0 if horizontal collapses.
    """
    if eye == "right":
        idx = _LANDMARK_RIGHT_EYE
    elif eye == "left":
        idx = _LANDMARK_LEFT_EYE
    else:
        raise ValueError(f"eye must be 'left' or 'right', got {eye!r}")

    top = landmarks_px[idx["top"], :2]
    bottom = landmarks_px[idx["bottom"], :2]
    outer = landmarks_px[idx["outer"], :2]
    inner = landmarks_px[idx["inner"], :2]

    vertical = math.hypot(top[0] - bottom[0], top[1] - bottom[1])
    horizontal = math.hypot(outer[0] - inner[0], outer[1] - inner[1])
    if horizontal < 1e-6:
        return 0.0
    return vertical / horizontal


def _compute_face_center(
    landmarks_px: Any, frame_w: int, frame_h: int,
) -> tuple[float, float]:
    """Normalized face center (cx, cy in [-1, +1]) from nose tip (1). Spec: PHASE2.md 2.1.

    Y axis follows image convention (top = -1, bottom = +1). Go-side mapper
    (Phase 2.4) can invert if its convention differs.
    """
    nose = landmarks_px[LANDMARK_NOSE_TIP, :2]
    return (
        (float(nose[0]) / frame_w) * 2.0 - 1.0,
        (float(nose[1]) / frame_h) * 2.0 - 1.0,
    )


# ---------------------------------------------------------------------------
# Detection result builder
# ---------------------------------------------------------------------------

def build_detection_message(
    seq: int,
    landmarks_px: Optional[Any],
    frame_shape: tuple[int, int, int],
    timestamp: float,
) -> dict[str, Any]:
    """Build the JSON detection dict per PHASE2.md Section 4.3.

    When landmarks_px is None (no face) all numeric fields are zero-filled
    so the Go side always sees a consistent schema (Phase 2.7 maps a
    sustained face_detected=False to the mouse-follow fallback).
    """
    h, w = frame_shape[:2]
    if landmarks_px is None:
        return {
            "type": "detection",
            "seq": seq,
            "timestamp": timestamp,
            "face_detected": False,
            "yaw": 0.0, "pitch": 0.0, "roll": 0.0,
            "ear_left": 0.0, "ear_right": 0.0,
            "face_center_x": 0.0, "face_center_y": 0.0,
        }

    yaw, pitch, roll = _compute_head_pose(landmarks_px, frame_shape)
    ear_left = _compute_ear(landmarks_px, eye="left")
    ear_right = _compute_ear(landmarks_px, eye="right")
    cx, cy = _compute_face_center(landmarks_px, w, h)

    return {
        "type": "detection",
        "seq": seq,
        "timestamp": timestamp,
        "face_detected": True,
        "yaw": yaw, "pitch": pitch, "roll": roll,
        "ear_left": ear_left, "ear_right": ear_right,
        "face_center_x": cx, "face_center_y": cy,
    }


# ---------------------------------------------------------------------------
# Frame receiver (Phase 2.1 stub)
# ---------------------------------------------------------------------------

def _frame_recv_loop(
    sub: Any, stop_event: threading.Event, log: logging.Logger,
) -> None:
    """Drain inbound frames from SUB; log size only in Phase 2.1.

    TODO Phase 2.5: parse base64 JPEG, decode to BGR, feed detector instead
    of (or alongside) cv2.VideoCapture. No ACK is sent in Phase 2.1.
    """
    import zmq  # local import: keeps the helper analyzable in isolation

    while not stop_event.is_set():
        try:
            msg = sub.recv_string()
        except zmq.Again:
            continue
        except zmq.ZMQError as e:
            if stop_event.is_set():
                return
            log.debug("SUB recv error: %s", e)
            continue
        log.debug(
            "Frame received: %d bytes (no-op, ack disabled in Phase 2.1)",
            len(msg),
        )


# ---------------------------------------------------------------------------
# FPS counter
# ---------------------------------------------------------------------------

class _FpsCounter:
    """Sliding-window FPS counter (default 1-second window)."""

    def __init__(self, window_sec: float = 1.0) -> None:
        """Empty window of ``window_sec`` seconds."""
        self._window_sec = float(window_sec)
        self._timestamps: list[float] = []

    def tick(self) -> None:
        """Record one frame timestamp and drop entries older than the window."""
        now = time.monotonic()
        self._timestamps.append(now)
        cutoff = now - self._window_sec
        self._timestamps = [t for t in self._timestamps if t >= cutoff]

    def fps(self) -> float:
        """Return current sliding-window FPS (0.0 if fewer than 2 ticks)."""
        if len(self._timestamps) < 2:
            return 0.0
        span = self._timestamps[-1] - self._timestamps[0]
        if span < 1e-6:
            return 0.0
        return (len(self._timestamps) - 1) / span


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main(argv: Optional[list[str]] = None) -> int:
    """Run the mp_server main loop and return the process exit code.

    Lifecycle: parse args -> ensure model -> bind PUB + connect SUB ->
    open camera -> build detector -> install signal handlers -> spawn
    SUB recv thread -> loop {read, detect, publish} -> graceful cleanup.
    Spec: docs/PHASE2.md Phase 2.1.
    """
    args = _parse_args(argv)
    log = _setup_logger(args.debug)

    log.info("GoTuber Phase 2.1 mp_server starting (docs/PHASE2.md 4.1/4.3)")
    log.info(
        "config: frame_port=%d detection_port=%d camera_id=%d model=%s debug=%s",
        args.frame_port, args.detection_port, args.camera_id,
        args.model_path, args.debug,
    )

    import cv2
    import mediapipe as mp
    import numpy as np
    import zmq

    # 1. Model
    model_path = Path(args.model_path)
    try:
        model_path = _ensure_model(model_path, not args.no_auto_download, log)
    except (FileNotFoundError, RuntimeError) as e:
        log.error("Model setup failed: %s", e)
        return 1

    zmq_ctx: Optional[zmq.Context] = None
    pub: Optional[zmq.Socket] = None
    sub: Optional[zmq.Socket] = None
    cap: Optional[cv2.VideoCapture] = None
    detector: Any = None
    recv_thread: Optional[threading.Thread] = None
    stop_event = threading.Event()
    fps = _FpsCounter()
    frames_published = 0
    exit_code = 0

    try:
        zmq_ctx = zmq.Context.instance()

        pub = zmq_ctx.socket(zmq.PUB)
        try:
            pub.bind(f"tcp://*:{args.detection_port}")
        except zmq.ZMQError as e:
            log.error("Failed to bind PUB port %d: %s", args.detection_port, e)
            return 1
        log.info("PUB bound to tcp://*:%d", args.detection_port)

        sub = zmq_ctx.socket(zmq.SUB)
        sub.setsockopt(zmq.SUBSCRIBE, b"")
        sub.setsockopt(zmq.RCVTIMEO, _SUB_RECV_TIMEOUT_MS)
        sub.connect(f"tcp://localhost:{args.frame_port}")
        log.info("SUB connected to tcp://localhost:%d", args.frame_port)

        cap = cv2.VideoCapture(args.camera_id)
        if not cap.isOpened():
            log.error("Cannot open camera id=%d", args.camera_id)
            return 1
        # frame.shape を使う (cap.get() の返す値と実フレームが異なる USB カメラがあるため)。
        # 1 フレームだけ読み捨てで shape 確定してもよいが、初回 read まで width/height は確定しない。
        # 代わりに毎フレーム frame.shape[1] / frame.shape[0] を真実とする (Phase 2.1 review Significant-2)。
        log.info("Camera %d opened", args.camera_id)

        detector = _create_landmarker(model_path, log)

        def _on_signal(signum: int, _frame: Any) -> None:
            try:
                signame = signal.Signals(signum).name
            except ValueError:
                signame = str(signum)
            log.info("Received signal %s, shutting down...", signame)
            stop_event.set()

        signal.signal(signal.SIGINT, _on_signal)
        # SIGTERM is not deliverable on Windows; registering unconditionally
        # lets Linux/macOS supervisors kill mp_server cleanly.
        if hasattr(signal, "SIGTERM"):
            signal.signal(signal.SIGTERM, _on_signal)

        recv_thread = threading.Thread(
            target=_frame_recv_loop,
            args=(sub, stop_event, log),
            name="mp-frame-recv",
            daemon=True,
        )
        recv_thread.start()

        seq = 0
        last_face_seen = time.monotonic()
        face_lost_warned = False

        while not stop_event.is_set():
            ok, frame = cap.read()
            if not ok or frame is None:
                log.warning("Camera read failed; sleeping briefly")
                if stop_event.wait(timeout=_CAMERA_READ_RETRY_SEC):
                    break
                continue

            timestamp = time.time()
            rgb = cv2.cvtColor(frame, cv2.COLOR_BGR2RGB)
            mp_image = mp.Image(image_format=mp.ImageFormat.SRGB, data=rgb)
            result = detector.detect(mp_image)

            if result.face_landmarks:
                # frame.shape[1] (width) / frame.shape[0] (height) を真実とする
                # (Phase 2.1 review Significant-2: cap.get() の値と実フレームが異なる USB カメラがある)
                h, w = frame.shape[:2]
                landmarks_px = _landmarks_to_pixels(
                    result.face_landmarks[0], w, h,
                )
                last_face_seen = time.monotonic()
                face_lost_warned = False
            else:
                landmarks_px = None

            msg = build_detection_message(
                seq=seq, landmarks_px=landmarks_px,
                frame_shape=frame.shape, timestamp=timestamp,
            )
            try:
                pub.send_string(json.dumps(msg))
            except zmq.ZMQError as e:
                log.error("PUB send failed: %s", e)
                exit_code = 1  # Process supervisor (systemd / Go-side spawner) needs to know this is a fatal failure, not a clean shutdown
                break

            seq += 1
            frames_published = seq
            fps.tick()

            if landmarks_px is None:
                elapsed = time.monotonic() - last_face_seen
                if elapsed > _FACE_LOST_WARN_SEC and not face_lost_warned:
                    log.warning(
                        "Face not detected for >%.1fs (continuing; seq=%d)",
                        elapsed, seq,
                    )
                    face_lost_warned = True

            if args.debug and seq % _FPS_DEBUG_INTERVAL == 0:
                log.debug(
                    "seq=%d fps=%.1f face=%s yaw=%.1f pitch=%.1f",
                    seq, fps.fps(), msg["face_detected"],
                    msg["yaw"], msg["pitch"],
                )

    except KeyboardInterrupt:
        log.info("KeyboardInterrupt received, shutting down...")
        stop_event.set()
    except Exception as e:  # noqa: BLE001
        log.exception("Fatal error: %s", e)
        exit_code = 1
    finally:
        log.info("Cleaning up...")
        # cap.release() takes no args; detector.close() takes no args; zmq
        # sockets accept linger=linger to drop pending messages immediately.
        for resource, name, kwargs in (
            (cap, "cap", {}),
            (detector, "detector", {}),
            (pub, "pub", {"linger": 0}),
            (sub, "sub", {"linger": 0}),
        ):
            if resource is None:
                continue
            closer = getattr(resource, "close", None) or getattr(
                resource, "release", None,
            )
            if closer is None:
                continue
            try:
                closer(**kwargs)
            except Exception as e:  # noqa: BLE001
                log.debug("%s.close: %s", name, e)
        if recv_thread is not None and recv_thread.is_alive():
            recv_thread.join(timeout=2.0)
        if zmq_ctx is not None:
            zmq_ctx.term()
        log.info(
            "mp_server exited (frames_published=%d avg_fps=%.1f)",
            frames_published, fps.fps(),
        )

    return exit_code


if __name__ == "__main__":
    sys.exit(main())