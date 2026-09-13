#!/usr/bin/env python3
"""Exercise the action's child-process isolation, not shell source text."""
import json
import os
from pathlib import Path
import signal
import shutil
import tarfile
import subprocess
import tempfile
import time
import unittest

ACTION = Path(__file__).resolve().parent


class IsolationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="sb-action-test-", dir="/tmp")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.workspace = self.root / "workspace"
        self.workspace.mkdir()
        self.caller = self.root / "caller"
        self.caller.mkdir()
        self.target = self.root / "target"
        self.target.mkdir()
        self.env = {"PATH": "/usr/bin:/bin", "HOME": str(self.root / "missing-home")}

    def command(self, kind, executable, *args):
        return ["/bin/bash", "-c", 'source "$1"; managed_python_command "$2" "$3" "$4" "$5" "${@:6}"',
                "test", str(ACTION / "managed-python.sh"), str(self.workspace), kind,
                str(self.target), str(executable), *args]

    def run_child(self, kind, executable, *args, env=None):
        return subprocess.run(self.command(kind, executable, *args), cwd=self.caller,
                              env=self.env | (env or {}), text=True, capture_output=True)

    def test_python_ignores_ambient_settings_and_keeps_network_configuration(self):
        hostile = {"UV_CONFIG_FILE": "missing.toml", "UV_WORKING_DIR": "/missing",
                   "UV_WORKING_DIRECTORY": "/missing", "UV_PYTHON": "/missing/python",
                   "UV_CACHE_DIR": "/missing/cache", "UV_INDEX_URL": "https://invalid.example",
                   "UV": "/missing/uv", "PIP_INDEX_URL": "https://invalid.example",
                   "PYTHONHOME": "/missing", "PYTHONPATH": str(self.caller),
                   "VIRTUAL_ENV": "/missing", "CONDA_PREFIX": "/missing",
                   "_CONDA_ROOT": "/missing", "__PYVENV_LAUNCHER__": "/missing",
                   "XDG_CACHE_HOME": "/missing", "TMPDIR": "/missing", "TMP": "/missing",
                   "TEMP": "/missing", "HTTPS_PROXY": "http://proxy.example:3128",
                   "https_proxy": "http://proxy.example:3128", "NO_PROXY": "localhost",
                   "SSL_CERT_FILE": "ca.pem", "SSL_CERT_DIR": "certs:/etc/ssl/certs",
                   "UV_SYSTEM_CERTS": "true", "UV_NATIVE_TLS": "true"}
        (self.caller / "json.py").write_text('raise RuntimeError("shadowed json")\n')
        code = "import json,os,sys; print(json.dumps(dict(cwd=os.getcwd(), env=dict(os.environ), isolated=sys.flags.isolated)))"
        result = self.run_child("python", "/usr/bin/python3", "-c", code, env=hostile)
        self.assertEqual(result.returncode, 0, result.stderr)
        got = json.loads(result.stdout)
        self.assertEqual(got["cwd"], str(self.target))
        self.assertEqual(got["env"]["PWD"], str(self.target))
        self.assertEqual(got["isolated"], 1)
        for key in hostile:
            if key.startswith(("UV", "PIP", "PYTHON", "VIRTUAL", "CONDA", "_CONDA", "__PYVENV")) and key not in ("UV_CACHE_DIR", "UV_SYSTEM_CERTS", "UV_NATIVE_TLS"):
                self.assertNotIn(key, got["env"])
        self.assertEqual(got["env"]["HTTPS_PROXY"], hostile["HTTPS_PROXY"])
        self.assertEqual(got["env"]["https_proxy"], hostile["https_proxy"])
        self.assertEqual(got["env"]["SSL_CERT_FILE"], str(self.caller / "ca.pem"))
        self.assertEqual(got["env"]["SSL_CERT_DIR"], str(self.caller / "certs") + ":/etc/ssl/certs")
        self.assertEqual(got["env"]["UV_CACHE_DIR"], str(self.workspace / "cache"))
        self.assertEqual(got["env"]["UV_SYSTEM_CERTS"], "true")
        scratch = Path(got["env"]["TMPDIR"])
        self.assertTrue(scratch.is_relative_to(self.workspace / "tmp"))
        self.assertFalse(scratch.exists())

    def test_absolute_inputs_work_from_deleted_caller_directory(self):
        result = subprocess.run(self.command("python", "/usr/bin/python3", "-c", "import os; print(os.getcwd())"),
                                cwd=self.caller, env=self.env, preexec_fn=lambda: os.rmdir(self.caller),
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), str(self.target))

    def test_uv_receives_no_config_and_explicit_install_directory(self):
        probe = self.root / "uv-probe"
        probe.write_text('#!/usr/bin/python3\nimport json,os,sys\nprint(json.dumps([sys.argv[1:],os.environ.get("UV_PYTHON_INSTALL_DIR")]))\n')
        probe.chmod(0o755)
        result = self.run_child("uv", probe, "python", "find", "3.12.13")
        self.assertEqual(result.returncode, 0, result.stderr)
        args, install_dir = json.loads(result.stdout)
        self.assertEqual(args, ["--no-config", "python", "find", "3.12.13"])
        self.assertEqual(install_dir, str(self.workspace / "python"))

    def test_entrypoint_ignores_ansible_config_and_uses_selected_venv(self):
        bin_dir = self.target / "bin"
        bin_dir.mkdir()
        probe = bin_dir / "ansible"
        probe.write_text('#!/usr/bin/python3\nimport json,os\nfrom pathlib import Path\nprint(json.dumps(dict(env=dict(os.environ), config=Path(os.environ["ANSIBLE_CONFIG"]).read_text())))\n')
        probe.chmod(0o755)
        result = self.run_child("entrypoint", probe, "--version", env={"ANSIBLE_CONFIG": "/missing", "ANSIBLE_HOME": "/missing", "PYTHONPATH": "/missing"})
        self.assertEqual(result.returncode, 0, result.stderr)
        got = json.loads(result.stdout)
        self.assertEqual(got["config"], "")
        self.assertNotIn("ANSIBLE_HOME", got["env"])
        self.assertEqual(got["env"]["PYTHONNOUSERSITE"], "1")
        self.assertEqual(got["env"]["PYTHONSAFEPATH"], "1")
        self.assertTrue(got["env"]["PATH"].startswith(str(bin_dir) + ":"))
        self.assertFalse(Path(got["env"]["ANSIBLE_CONFIG"]).exists())

    @unittest.skipUnless(os.environ.get("SB_TEST_UV_BINARY"), "set SB_TEST_UV_BINARY for real uv acceptance")
    def test_real_uv_ignores_ambient_config_and_storage(self):
        uv = Path(os.environ["SB_TEST_UV_BINARY"]).resolve()
        (self.caller / "uv.toml").write_text("invalid = [")
        (self.caller / ".python-version").write_text("0.0.0")
        (self.caller / ".env").write_text("UV_PYTHON=/missing\n")
        (self.caller / ".venv").mkdir()
        hostile = {"UV_CONFIG_FILE": "/missing", "UV_WORKING_DIR": "/missing",
                   "UV_WORKING_DIRECTORY": "/missing", "UV_CACHE_DIR": "/missing",
                   "UV_PROJECT_ENVIRONMENT": "/missing", "UV_PYTHON": "/missing",
                   "PYTHONHOME": "/missing", "PYTHONPATH": "/missing",
                   "VIRTUAL_ENV": "/missing", "XDG_CACHE_HOME": "/missing", "TMPDIR": "/missing"}
        venv = self.root / "venv"
        result = self.run_child("uv", uv, "venv", "--python", shutil.which("python3"),
                                "--no-project", "--no-python-downloads", str(venv), env=hostile)
        self.assertEqual(result.returncode, 0, result.stderr)
        result = self.run_child("uv", uv, "pip", "check", "--python", str(venv / "bin/python"), env=hostile)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(list((self.workspace / "tmp").iterdir()), [])

    def test_python_preserves_stdin(self):
        result = subprocess.run(self.command("python", "/usr/bin/python3", "-"), cwd=self.caller,
                                env=self.env, input='print("stdin preserved")', text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "stdin preserved\n")

    def test_failure_keeps_stderr_and_removes_scratch(self):
        result = self.run_child("python", "/usr/bin/python3", "-c", 'import sys; print("expected failure", file=sys.stderr); sys.exit(17)')
        self.assertEqual(result.returncode, 17, result.stderr)
        self.assertIn("expected failure", result.stderr)
        self.assertEqual(list((self.workspace / "tmp").iterdir()), [])

    def test_termination_removes_scratch(self):
        for sig in (signal.SIGTERM, signal.SIGINT):
            with self.subTest(signal=sig):
                marker = self.root / f"ready-{sig}"
                code = f'import pathlib,time; pathlib.Path({str(marker)!r}).touch(); time.sleep(60)'
                child = subprocess.Popen(self.command("python", "/usr/bin/python3", "-c", code),
                                         cwd=self.caller, env=self.env, start_new_session=True,
                                         stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
                try:
                    deadline = time.monotonic() + 5
                    while not marker.exists() and child.poll() is None and time.monotonic() < deadline:
                        time.sleep(0.01)
                    self.assertTrue(marker.exists(), "child did not start")
                    os.killpg(child.pid, sig)
                    child.communicate(timeout=5)
                    self.assertNotEqual(child.returncode, 0)
                    self.assertEqual(list((self.workspace / "tmp").iterdir()), [])
                finally:
                    if child.poll() is None:
                        os.killpg(child.pid, signal.SIGKILL)
                        child.wait()
                    child.stderr.close()


class ActionTests(unittest.TestCase):
    def test_dry_run_is_independent_of_ambient_configuration(self):
        with tempfile.TemporaryDirectory(prefix="sb-action-dry-run-", dir="/tmp") as tmp:
            root = Path(tmp)
            repo = root / "saltbox"
            (repo / "requirements").mkdir(parents=True)
            (repo / ".github").mkdir()
            (repo / ".python-version").write_text("3.12.13\n")
            (repo / ".uv-version").write_text("0.12.7\n")
            (repo / ".github/renovate.json").write_text('{"constraints": {"uv": "0.12.7"}}\n')
            (repo / "requirements/requirements-saltbox.in").write_text("fixture==1.0\n")
            (repo / "requirements/requirements-saltbox.txt").write_text("old lock\n")
            for args in (["init", "-b", "master"], ["add", "."],
                         ["-c", "user.name=fixture", "-c", "user.email=fixture@example.test", "commit", "-m", "test: create fixture"]):
                subprocess.run(["git", "-C", str(repo), *args], check=True, capture_output=True)
            original = subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD"], text=True)
            uv = root / "uv"
            uv.write_text(UV_FIXTURE)
            uv.chmod(0o755)
            archive = root / "uv.tar.gz"
            with tarfile.open(archive, "w:gz") as tar:
                tar.add(uv, arcname="uv-x86_64-unknown-linux-gnu/uv")
            bin_dir = root / "bin"
            bin_dir.mkdir()
            curl = bin_dir / "curl"
            curl.write_text('#!/bin/bash\nwhile (($#)); do if [[ $1 == --output ]]; then cp "$TEST_UV_ARCHIVE" "$2"; shift 2; else shift; fi; done\nprintf 200\n')
            curl.chmod(0o755)
            results = []
            for hostile in (False, True):
                caller = root / ("hostile" if hostile else "clean")
                caller.mkdir()
                if hostile:
                    (caller / "uv.toml").write_text("invalid = [")
                    (caller / ".python-version").write_text("0.0.0")
                log = root / f"calls-{hostile}.jsonl"
                env = {"PATH": str(bin_dir) + ":/usr/bin:/bin", "HOME": str(root),
                       "GITHUB_ACTION_PATH": str(ACTION), "UV_VERSION": "0.12.10",
                       "PYTHON_MINOR": "3.12", "SB_GO_RELEASE": "0.0.110", "DRY_RUN": "true",
                       "SALTBOX_CLONE_URL": str(repo), "TEST_UV_ARCHIVE": str(archive), "TEST_UV_LOG": str(log),
                       "SSL_CERT_FILE": "ca.pem", "TEST_CERT_FILE": str(caller / "ca.pem")}
                if hostile:
                    env |= {"UV_CONFIG_FILE": "/missing/config", "UV_WORKING_DIR": "/missing/cwd",
                            "UV_CACHE_DIR": "/missing/cache", "PYTHONHOME": "/missing/python",
                            "PYTHONPATH": "/missing/path", "TMPDIR": "/missing/tmp"}
                result = subprocess.run(["bash", str(ACTION / "update.sh")], cwd=caller, env=env,
                                        capture_output=True, text=True, timeout=30)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("+3.12.14", result.stdout)
                self.assertIn("+0.12.10", result.stdout)
                self.assertIn('+{"constraints": {"uv": "0.12.10"}}', result.stdout)
                self.assertNotIn("sb-go-python-action-", result.stdout)
                calls = [json.loads(line) for line in log.read_text().splitlines()]
                self.assertEqual(sum(call["args"][:2] == ["pip", "compile"] for call in calls), 2)
                self.assertEqual(subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD"], text=True), original)
                results.append(result.stdout)
            self.assertEqual(results[0], results[1])


UV_FIXTURE = r'''#!/usr/bin/python3
import json, os, pathlib, sys
args = sys.argv[1:]
if not args or args.pop(0) != "--no-config":
    sys.exit("managed uv invocation did not disable ambient configuration")
for key in ["UV_CONFIG_FILE", "UV_WORKING_DIR", "PYTHONHOME", "PYTHONPATH", "UV_VERSION", "PYTHON_MINOR"]:
    if key in os.environ:
        sys.exit("ambient setting reached managed uv: " + key)
if os.environ.get("SSL_CERT_FILE") != os.environ["TEST_CERT_FILE"]:
    sys.exit("relative certificate resolved from the wrong directory")
with open(os.environ["TEST_UV_LOG"], "a") as log:
    log.write(json.dumps(dict(args=args, cwd=os.getcwd())) + "\n")
python_script = '#!/bin/bash\nif [[ ${1:-} == -I ]]; then shift; fi\nif [[ ${1:-} == --version ]]; then echo "Python 3.12.14"; else exec /usr/bin/python3 -I "$@"; fi\n'
def executable(path, content):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)
    path.chmod(0o755)
if args == ["--version"]:
    print("uv 0.12.10")
elif args[:2] == ["python", "list"]:
    print(json.dumps([dict(implementation="cpython", os="linux", arch="x86_64", libc="gnu", variant="default", version="3.12.14", version_parts=dict(major=3,minor=12,patch=14))]))
elif args[:2] == ["python", "install"]:
    executable(pathlib.Path(args[args.index("--install-dir")+1]) / "bin/python3", python_script)
elif args[:2] == ["python", "find"]:
    print(pathlib.Path(os.environ["UV_PYTHON_INSTALL_DIR"]) / "bin/python3")
elif args[:2] == ["pip", "compile"]:
    pathlib.Path(args[args.index("--output-file")+1]).write_text("# stable fixture lock\nfixture==1.0\n")
elif args[0] == "venv":
    venv = pathlib.Path(args[-1])
    executable(venv / "bin/python", python_script)
    for name in ["ansible", "certbot", "apprise"]:
        executable(venv / "bin" / name, "#!/bin/sh\nexit 0\n")
elif args[:2] not in (["pip", "sync"], ["pip", "check"]):
    sys.exit("unexpected uv command: " + repr(args))
'''

if __name__ == "__main__":
    unittest.main()
