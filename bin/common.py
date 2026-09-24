"""Shared helpers for aitools, skills, and MCP commands."""

from __future__ import annotations

import json
import os
import platform
import shutil
import subprocess
import sys
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path

SCRIPT_PATH = Path(__file__).resolve()
if SCRIPT_PATH.name == "common.py":
    SCRIPT_PATH = SCRIPT_PATH.parent / "aitools"
REPO_ROOT = Path(__file__).resolve().parent.parent
SKILLS_ROOT = REPO_ROOT / "skills"
MCP_ROOT = REPO_ROOT / "mcp"
CLONE_URL = "https://github.com/claudio-silva/ai-tools.git"

PLATFORM_ORDER = ("cursor", "codex", "claude-code", "devin")

FOLDER_PLATFORMS = {
    "Shared": ("cursor", "codex", "claude-code", "devin"),
    "Cursor": ("cursor",),
    "Codex": ("codex",),
    "Claude": ("claude-code",),
    "Devin": ("devin",),
}

SKIP_DIR_NAMES = {"__pycache__", ".git"}
VARIABLES = ("SKILLS_DIR", "SKILL_DIR", "PLATFORM_HOME", "CODEX_HOME", "HOME")


class SkillError(Exception):
    """A user-facing failure. The message is printed as-is."""


@dataclass(frozen=True)
class Skill:
    name: str
    folder: str
    path: Path
    platforms: tuple[str, ...]


@dataclass(frozen=True)
class Target:
    platform: str
    scope: str
    project: Path | None
    skills_dir: Path
    platform_home: Path
    skill_dir: Path

    @property
    def project_key(self) -> str:
        return str(self.project.resolve()) if self.project else ""

    def key(self, skill_name: str) -> tuple[str, str, str, str]:
        return (skill_name, self.platform, self.scope, self.project_key)


@dataclass
class Action:
    kind: str  # copy, remove, keep
    path: Path
    src: Path | None = None
    note: str = ""


@dataclass
class Plan:
    skill: Skill
    target: Target
    actions: list[Action] = field(default_factory=list)
    external_files: list[str] = field(default_factory=list)


def require_macos_arm() -> None:
    if sys.platform != "darwin" or platform.machine() != "arm64":
        die("aitools runs on macOS Apple Silicon only")


def home() -> Path:
    return Path.home()


def codex_home() -> Path:
    raw = os.environ.get("CODEX_HOME", str(home() / ".codex"))
    return Path(raw).expanduser()


def state_file() -> Path:
    base = os.environ.get("XDG_STATE_HOME")
    root = Path(base).expanduser() if base else home() / ".local" / "state"
    return root / "ai-tools" / "installs.json"


def display_link(path: Path) -> str:
    """Show a path as stored, without following a symlink at the end."""
    expanded = path.expanduser()
    try:
        rel = expanded.relative_to(home())
    except ValueError:
        return str(expanded)
    text = rel.as_posix()
    return "~" if text == "." else "~/" + text


def display(path: Path) -> str:
    resolved = path.expanduser().resolve()
    try:
        rel = resolved.relative_to(home().resolve())
    except ValueError:
        return str(resolved)
    text = rel.as_posix()
    return "~" if text == "." else "~/" + text


def die(message: str) -> None:
    raise SkillError(message)


def load_state() -> dict:
    path = state_file()
    if not path.exists():
        return {"version": 1, "installs": []}
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        die(f"{path}: {exc}")
    if not isinstance(data, dict) or data.get("version") != 1 or not isinstance(data.get("installs"), list):
        die(f"{path}: unrecognized install record")
    return data


def save_state(data: dict) -> None:
    path = state_file()
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(".json.tmp")
    tmp.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
    tmp.replace(path)


def entry_kind(entry: dict) -> str:
    return entry.get("kind", "skill")


def entry_key(entry: dict) -> tuple[str, str, str, str]:
    name = entry.get("skill") or entry.get("name") or ""
    return (name, entry["platform"], entry["scope"], entry.get("project") or "")


def trash_bin() -> str:
    found = shutil.which("trash")
    if not found:
        die("trash is required on PATH (macOS /usr/bin/trash, or the trash CLI)")
    return found


def trash_path(path: Path) -> None:
    # macOS /usr/bin/trash treats "--" as a filename, not an end-of-options
    # marker. Destinations are absolute, so a leading dash is not a concern.
    result = subprocess.run(
        [trash_bin(), str(path)],
        check=False,
        text=True,
        capture_output=True,
    )
    if result.returncode != 0:
        detail = (result.stderr or result.stdout).strip()
        die(f"trash {path} failed: {detail or 'exit ' + str(result.returncode)}")


def present(path: Path) -> bool:
    return path.is_symlink() or path.exists()


def platform_paths(platform: str, scope: str, project: Path | None) -> tuple[Path, Path]:
    """Return (skills_dir, platform_home) for a platform and scope."""
    if scope == "local":
        if project is None:
            die("local scope needs a project directory")
        local = {
            "cursor": (project / ".cursor" / "skills", project / ".cursor"),
            "codex": (project / ".agents" / "skills", project / ".codex"),
            "claude-code": (project / ".claude" / "skills", project / ".claude"),
            "devin": (project / ".devin" / "skills", project / ".devin"),
        }
        return local[platform]
    global_paths = {
        "cursor": (home() / ".cursor" / "skills", home() / ".cursor"),
        "codex": (codex_home() / "skills", codex_home()),
        "claude-code": (home() / ".claude" / "skills", home() / ".claude"),
        "devin": (home() / ".config" / "devin" / "skills", home() / ".config" / "devin"),
    }
    return global_paths[platform]


def make_target(platform: str, scope: str, project: Path | None, skill_name: str) -> Target:
    skills_dir, platform_home = platform_paths(platform, scope, project)
    return Target(platform, scope, project, skills_dir, platform_home, skills_dir / skill_name)


def still_needed(installs: list[dict], path: Path, skip_keys: set[tuple[str, str, str, str]]) -> bool:
    text = str(path.resolve())
    for entry in installs:
        if entry_kind(entry) != "skill":
            continue
        if entry_key(entry) in skip_keys:
            continue
        skill_dir = entry.get("skillDir", "")
        if text == skill_dir or text in entry.get("externalFiles", []):
            return True
        if skill_dir and text.startswith(skill_dir.rstrip("/") + "/"):
            return True
    return False


def find_entry(
    installs: list[dict],
    key: tuple[str, str, str, str],
    kind: str = "skill",
) -> dict | None:
    for entry in installs:
        if entry_kind(entry) != kind:
            continue
        if entry_key(entry) == key:
            return entry
    return None


def format_version(mtime: float) -> str:
    return datetime.fromtimestamp(mtime).strftime("v%y%m%d%H%M")


def apply_plan(plan: Plan, dry_run: bool) -> None:
    if dry_run:
        return
    removes = [action for action in plan.actions if action.kind == "remove"]
    copies = [action for action in plan.actions if action.kind == "copy"]
    skill_root = plan.target.skill_dir.resolve()
    outside = [action for action in removes if not _inside(action.path, skill_root) or action.path.resolve() == skill_root]
    inside = [action for action in removes if _inside(action.path, skill_root) and action.path.resolve() != skill_root]
    for action in removal_order(outside):
        if present(action.path):
            trash_path(action.path)
    for action in copies:
        dest = action.path
        if dest.is_symlink():
            trash_path(dest)
        elif dest.exists() and dest.is_dir():
            die(f"destination is a directory: {display(dest)}")
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(action.src, dest)
    for action in removal_order(inside):
        if present(action.path):
            trash_path(action.path)


def removal_order(actions: list[Action]) -> list[Action]:
    files = [action for action in actions if action.path.is_symlink() or not action.path.is_dir()]
    directories = [action for action in actions if not action.path.is_symlink() and action.path.is_dir()]
    directories.sort(key=lambda action: len(action.path.resolve().parts), reverse=True)
    return files + directories


def _inside(path: Path, root: Path) -> bool:
    try:
        path.resolve().relative_to(root)
    except ValueError:
        return False
    return True


def record_skill_install(state: dict, plan: Plan) -> None:
    key = plan.target.key(plan.skill.name)
    state["installs"] = [
        entry
        for entry in state["installs"]
        if not (entry_kind(entry) == "skill" and entry_key(entry) == key)
    ]
    state["installs"].append(
        {
            "kind": "skill",
            "skill": plan.skill.name,
            "folder": plan.skill.folder,
            "platform": plan.target.platform,
            "scope": plan.target.scope,
            "project": plan.target.project_key,
            "skillDir": str(plan.target.skill_dir.resolve()),
            "externalFiles": plan.external_files,
        }
    )


def drop_install(state: dict, name: str, target: Target, kind: str = "skill") -> None:
    key = target.key(name)
    state["installs"] = [
        entry
        for entry in state["installs"]
        if not (entry_kind(entry) == kind and entry_key(entry) == key)
    ]


def selected_scopes(args, default: tuple[str, ...]) -> list[str]:
    chosen: list[str] = []
    if args.scope_global:
        chosen.append("global")
    if args.local:
        chosen.append("local")
    return chosen or list(default)


def selected_platforms(values: list[str]) -> list[str] | None:
    if not values:
        return None
    chosen: list[str] = []
    for value in values:
        for part in value.split(","):
            name = part.strip()
            if not name:
                continue
            if name not in PLATFORM_ORDER:
                die(f"unknown platform {name!r}; expected {', '.join(PLATFORM_ORDER)}")
            if name not in chosen:
                chosen.append(name)
    return chosen


def project_dir(args) -> Path:
    raw = args.directory if args.directory is not None else Path.cwd()
    path = raw.expanduser().resolve()
    if not path.is_dir():
        die(f"project directory does not exist: {path}")
    return path


@dataclass
class InstalledGroup:
    scope: str
    platform: str
    location: Path
    rows: list[str]


def installed_scopes(args) -> tuple[list[str], Path | None]:
    scopes = selected_scopes(args, ("global", "local"))
    if args.directory is not None and "local" not in scopes:
        die("--directory is used with --local")
    project = project_dir(args) if "local" in scopes else None
    return scopes, project


def print_about(
    name: str,
    kind: str,
    fields: list[tuple[str, str]],
    installs: list[tuple[str, str, str]],
    text: str,
) -> None:
    import textwrap

    print(_style("1", name) + "  " + _style("2", kind))
    print()
    label = max(len(key) for key, _ in fields) + 2
    for key, value in fields:
        print("  " + _style("2", key.ljust(label)) + value)
    print()
    print("  " + _style("1;4", "Installed"))
    if not installs:
        print("    " + _style("2", "not installed"))
    else:
        for scope in ("global", "local"):
            rows = [(p, m) for s, p, m in installs if s == scope]
            if not rows:
                continue
            heading = "Global" if scope == "global" else "Project  " + _style("2", display(Path.cwd()))
            print("    " + heading)
            for platform, mark in rows:
                print(f"      {mark} {_style('36', platform)}")
    if text:
        print()
        for line in textwrap.wrap(text, width=74):
            print("  " + line)


def _style(code: str, text: str) -> str:
    if not sys.stdout.isatty() or os.environ.get("NO_COLOR"):
        return text
    return f"\x1b[{code}m{text}\x1b[0m"


def render_installed(
    sections: list[tuple[str, list[InstalledGroup]]],
    scopes: list[str],
    project: Path | None,
) -> None:
    width = max(len(name) for name in PLATFORM_ORDER)
    first = True
    for scope in scopes:
        if not first:
            print()
        first = False
        if scope == "global":
            print(_style("1", "Global"))
        else:
            print(_style("1", "Project") + "  " + _style("2", display(project)))
        printed = False
        for title, groups in sections:
            filled = [g for g in groups if g.scope == scope and g.rows]
            if not filled:
                continue
            print()
            print("  " + _style("1;4", title))
            for group in filled:
                print(f"    {_style('36', group.platform.ljust(width))}  {_style('2', display(group.location))}")
                for row in group.rows:
                    print(f"      {row}")
            printed = True
        if not printed:
            print("  " + _style("2", "nothing installed"))


def atomic_write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(text, encoding="utf-8")
    tmp.replace(path)


def load_json_object(path: Path) -> dict:
    if not path.exists():
        return {}
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        die(f"{display(path)}: {exc}")
    if not isinstance(data, dict):
        die(f"{display(path)}: expected a JSON object")
    return data
