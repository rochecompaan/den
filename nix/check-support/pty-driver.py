#!/usr/bin/env python3
import fcntl
import os
import pty
import signal
import struct
import subprocess
import sys
import termios
import time

runner, host_root = sys.argv[1:]
logical_root = os.environ["DEN_PTY_LOGICAL_ROOT"]

def wait_file(path, nonempty=False):
    for _ in range(200):
        if os.path.exists(path) and (not nonempty or os.path.getsize(path) > 0):
            return
        time.sleep(0.025)
    raise RuntimeError("PTY fixture readiness timeout")

def start():
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
    process = subprocess.Popen([runner], stdin=slave, stdout=slave, stderr=slave,
                               env=env, preexec_fn=session, close_fds=True)
    os.close(slave)
    wait_file(os.path.join(host_root, "pty.ready"))
    wait_file(os.path.join(host_root, "pty.pid"), True)
    child = int(open(os.path.join(host_root, "pty.pid"), encoding="utf-8").read())
    ps = "/bin/ps" if os.path.exists("/bin/ps") else "ps"
    parent = int(subprocess.check_output([ps, "-o", "ppid=", "-p", str(child)], text=True).strip())
    if os.tcgetpgrp(master) != os.getpgid(child):
        raise RuntimeError("fake Claude is not the PTY foreground group")
    return process, master, parent, child

def finish(process, master, expected):
    code = process.wait(timeout=10)
    os.close(master)
    if code != expected:
        raise RuntimeError(f"unexpected wrapper status {code}, expected {expected}")

for sig, expected, marker in (
    (signal.SIGINT, 41, "INT"),
    (signal.SIGTERM, 42, "TERM"),
    (signal.SIGHUP, 43, "HUP"),
    (signal.SIGQUIT, 44, "QUIT"),
):
    process, master, parent, _ = start()
    os.kill(parent, sig)
    finish(process, master, expected)
    data = open(os.path.join(host_root, "pty.signals"), encoding="utf-8").read()
    if marker not in data:
        raise RuntimeError("terminating signal was not observed")

process, master, parent, child = start()
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
finish(process, master, 42)
