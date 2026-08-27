#!/usr/bin/env python3
import fcntl
import os
import pty
import signal
import struct
import subprocess
import sys
import termios
import threading
import time

CAPTURE_LIMIT = 256 * 1024
DIAGNOSTIC_CAPTURE_LIMIT = 32 * 1024
STAGING_FILES = (
    "DEN_FAKE_FENCE_MARKER",
    "DEN_FAKE_FENCE_LOG",
    "DEN_FAKE_FENCE_ARGV_LOG",
    "DEN_FAKE_AGENT_LOG",
    "DEN_FAKE_REPOWOLF_LOG",
)

runner, host_root = sys.argv[1:]
logical_root = os.environ["DEN_PTY_LOGICAL_ROOT"]
latest_capture = bytearray()
latest_capture_lock = threading.Lock()
latest_staging_before = {}


def wait_file(path, nonempty=False):
    for _ in range(200):
        if os.path.exists(path) and (not nonempty or os.path.getsize(path) > 0):
            return
        time.sleep(0.025)
    raise RuntimeError("PTY fixture readiness timeout")


def snapshot_file(path, marker=False):
    try:
        size = os.path.getsize(path)
    except FileNotFoundError:
        return {"exists": "no", "size": "-", "invoked": "-"}
    except OSError:
        return {"exists": "error", "size": "-", "invoked": "-"}

    snapshot = {"exists": "yes", "size": str(size), "invoked": "-"}
    if marker:
        try:
            with open(path, encoding="utf-8", errors="replace") as file:
                snapshot["invoked"] = str(sum(line.rstrip("\n") == "invoked" for line in file))
        except OSError:
            snapshot["invoked"] = "error"
    return snapshot


def snapshot_staging():
    snapshots = {}
    for name in STAGING_FILES:
        path = os.environ.get(name)
        if path:
            snapshots[name] = snapshot_file(path, name == "DEN_FAKE_FENCE_MARKER")
    return snapshots


def drain_master(master, capture):
    while True:
        try:
            chunk = os.read(master, 4096)
        except OSError:
            return
        if not chunk:
            return
        with latest_capture_lock:
            capture.extend(chunk)
            if len(capture) > CAPTURE_LIMIT:
                del capture[:-CAPTURE_LIMIT]


def start():
    global latest_capture, latest_staging_before

    for name in ("pty.ready", "pty.pid", "pty.signals"):
        try:
            os.unlink(os.path.join(host_root, name))
        except FileNotFoundError:
            pass
    master, slave = pty.openpty()

    def session():
        os.setsid()
        fcntl.ioctl(slave, termios.TIOCSCTTY, 0)

    env = os.environ.copy()
    env.update({
        "DEN_FAKE_PROCESS_MODE": "pty-wait",
        "DEN_FAKE_READY_FILE": logical_root + "/pty.ready",
        "DEN_FAKE_PROCESS_PID_FILE": logical_root + "/pty.pid",
        "DEN_FAKE_SIGNAL_LOG": logical_root + "/pty.signals",
    })
    latest_staging_before = snapshot_staging()
    process = subprocess.Popen([runner], stdin=slave, stdout=slave, stderr=slave,
                               env=env, preexec_fn=session, close_fds=True)
    os.close(slave)
    capture = bytearray()
    with latest_capture_lock:
        latest_capture = capture
    drain = threading.Thread(target=drain_master, args=(master, capture), daemon=True)
    drain.start()
    wait_file(os.path.join(host_root, "pty.ready"))
    wait_file(os.path.join(host_root, "pty.pid"), True)
    child = int(open(os.path.join(host_root, "pty.pid"), encoding="utf-8").read())
    ps = "/bin/ps" if os.path.exists("/bin/ps") else "ps"
    parent = int(subprocess.check_output([ps, "-o", "ppid=", "-p", str(child)], text=True).strip())
    if os.tcgetpgrp(master) != os.getpgid(child):
        raise RuntimeError("fake Claude is not the PTY foreground group")
    return process, master, parent, child, drain


def finish(process, master, expected, drain):
    code = process.wait(timeout=10)
    drain.join(timeout=2)
    os.close(master)
    if code != expected:
        raise RuntimeError(f"unexpected wrapper status {code}, expected {expected}")


def dump_diagnostics():
    sys.stderr.write("pty-driver diagnostics: begin\n")
    for name in STAGING_FILES:
        if name not in latest_staging_before:
            continue
        before = latest_staging_before[name]
        after = snapshot_file(os.environ[name], name == "DEN_FAKE_FENCE_MARKER")
        sys.stderr.write(
            "pty-driver staging: "
            f"{name} before-exists={before['exists']} before-size={before['size']} "
            f"before-invoked={before['invoked']} after-exists={after['exists']} "
            f"after-size={after['size']} after-invoked={after['invoked']}\n"
        )
    with latest_capture_lock:
        output = bytes(latest_capture[-DIAGNOSTIC_CAPTURE_LIMIT:])
    sys.stderr.write("pty-driver captured output: begin\n")
    if output:
        decoded = output.decode(errors="replace")
        sys.stderr.write(decoded)
        if not decoded.endswith("\n"):
            sys.stderr.write("\n")
    sys.stderr.write("pty-driver captured output: end\n")
    sys.stderr.write("pty-driver diagnostics: end\n")
    sys.stderr.flush()


def main():
    for sig, expected, marker in (
        (signal.SIGINT, 41, "INT"),
        (signal.SIGTERM, 42, "TERM"),
        (signal.SIGHUP, 43, "HUP"),
        (signal.SIGQUIT, 44, "QUIT"),
    ):
        process, master, parent, _, drain = start()
        os.kill(parent, sig)
        finish(process, master, expected, drain)
        data = open(os.path.join(host_root, "pty.signals"), encoding="utf-8").read()
        if marker not in data:
            raise RuntimeError("terminating signal was not observed")

    process, master, parent, child, drain = start()
    fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
    for _ in range(100):
        if os.path.exists(os.path.join(host_root, "pty.signals")) and "WINCH" in open(os.path.join(host_root, "pty.signals"), encoding="utf-8").read():
            break
        time.sleep(0.025)
    else:
        raise RuntimeError("resize was not observed")
    foreground = os.tcgetpgrp(master)
    os.killpg(foreground, signal.SIGTSTP)
    time.sleep(0.1)
    os.killpg(foreground, signal.SIGCONT)
    for _ in range(100):
        data = open(os.path.join(host_root, "pty.signals"), encoding="utf-8").read()
        if "TSTP" in data and "CONT" in data:
            break
        time.sleep(0.025)
    else:
        raise RuntimeError("job-control signals were not observed")
    os.kill(parent, signal.SIGTERM)
    finish(process, master, 42, drain)


try:
    main()
except Exception:
    try:
        dump_diagnostics()
    except BaseException:
        try:
            sys.stderr.write("pty-driver diagnostics: unavailable\n")
            sys.stderr.flush()
        except BaseException:
            pass
    raise
