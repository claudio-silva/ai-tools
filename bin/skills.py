"""Install, update, uninstall, and list skills from this repository."""

from __future__ import annotations

import argparse
import os
import textwrap
from collections import Counter
from pathlib import Path

from common import (
    FOLDER_PLATFORMS,
    PLATFORM_ORDER,
    REPO_ROOT,
    SKIP_DIR_NAMES,
    SKILLS_ROOT,
    VARIABLES,
    Action,
    InstalledGroup,
    Plan,
    Skill,
    Target,
    apply_plan,
    drop_install,
    die,
    display,
    entry_kind,
    find_entry,
    format_version,
    home,
    installed_scopes,
    print_about,
    make_target,
    platform_paths,
    present,
    project_dir,
    record_skill_install,
    render_installed,
    selected_platforms,
    selected_scopes,
    still_needed,
    trash_path,
)


def load_manifest(skill: Skill) -> dict:
    path = skill.path / "manifest.json"
    try:
        data = _read_json(path)
    except Exception:
        raise
    if not isinstance(data, dict):
        die(f"{path.relative_to(REPO_ROOT)}: manifest must be an object")
    extra = set(data) - {"install", "remove"}
    if extra:
        die(f"{path.relative_to(REPO_ROOT)}: unknown keys: {', '.join(sorted(extra))}")
    install = data.get("install")
    if not isinstance(install, list) or not install:
        die(f"{path.relative_to(REPO_ROOT)}: install must be a non-empty list")
    remove = data.get("remove", [])
    if not isinstance(remove, list) or not all(isinstance(item, str) for item in remove):
        die(f"{path.relative_to(REPO_ROOT)}: remove must be a list of paths")
    for item in install:
        if isinstance(item, str):
            if not item:
                die(f"{path.relative_to(REPO_ROOT)}: empty install path")
        elif isinstance(item, dict):
            if set(item) != {"from", "to"} or not isinstance(item["from"], str) or not isinstance(item["to"], str):
                die(f"{path.relative_to(REPO_ROOT)}: install objects need string from and to")
            if not item["from"] or not item["to"]:
                die(f"{path.relative_to(REPO_ROOT)}: empty from or to")
        else:
            die(f"{path.relative_to(REPO_ROOT)}: install entries must be strings or objects")
    return {"install": install, "remove": remove}


def _read_json(path: Path) -> dict:
    import json

    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        die(f"{path.relative_to(REPO_ROOT)}: {exc}")
        raise


def discover_skills() -> list[Skill]:
    if not SKILLS_ROOT.is_dir():
        die(f"skills directory not found: {SKILLS_ROOT}")
    found: list[Skill] = []
    names: dict[str, Path] = {}
    for folder in sorted(SKILLS_ROOT.iterdir(), key=lambda p: p.name):
        if not folder.is_dir() or folder.name.startswith("."):
            continue
        platforms = FOLDER_PLATFORMS.get(folder.name)
        if platforms is None:
            known = ", ".join(FOLDER_PLATFORMS)
            die(f"unknown platform folder skills/{folder.name}; expected one of: {known}")
        for skill_dir in sorted(folder.iterdir(), key=lambda p: p.name):
            if not skill_dir.is_dir() or skill_dir.name.startswith("."):
                continue
            if not (skill_dir / "SKILL.md").is_file():
                die(f"{skill_dir.relative_to(REPO_ROOT)} is missing SKILL.md")
            if not (skill_dir / "manifest.json").is_file():
                die(f"{skill_dir.relative_to(REPO_ROOT)} is missing manifest.json")
            if skill_dir.name in names:
                die(
                    f"duplicate skill name {skill_dir.name}: "
                    f"{names[skill_dir.name].relative_to(REPO_ROOT)} and "
                    f"{skill_dir.relative_to(REPO_ROOT)}"
                )
            names[skill_dir.name] = skill_dir
            found.append(Skill(skill_dir.name, folder.name, skill_dir, platforms))
    return found


def variables_for(target: Target) -> dict[str, str]:
    from common import codex_home

    return {
        "SKILL_DIR": str(target.skill_dir),
        "SKILLS_DIR": str(target.skills_dir),
        "PLATFORM_HOME": str(target.platform_home),
        "CODEX_HOME": str(codex_home()),
        "HOME": str(home()),
    }


def expand_vars(text: str, variables: dict[str, str], where: str) -> str:
    out = text
    for token in VARIABLES:
        out = out.replace("$" + token, variables[token])
    if "$" in out:
        for token in text.replace("$$", "").split("$")[1:]:
            name = []
            for ch in token:
                if ch.isalnum() or ch == "_":
                    name.append(ch)
                else:
                    break
            ident = "".join(name)
            if ident and ident not in VARIABLES:
                die(f"{where}: unknown variable ${ident}")
    trailing_slash = out.endswith("/")
    expanded = Path(out).expanduser()
    if not expanded.is_absolute():
        die(f"{where}: {text} must expand to an absolute path")
    rendered = str(expanded)
    if trailing_slash and not rendered.endswith("/"):
        rendered += "/"
    return rendered


def allowed_roots(target: Target) -> list[Path]:
    from common import codex_home

    roots = [
        target.skills_dir,
        target.platform_home,
        codex_home(),
        home() / ".cursor",
        home() / ".codex",
        home() / ".claude",
        home() / ".config" / "devin",
        home() / ".agents",
    ]
    if target.project is not None:
        roots.append(target.project)
    return roots


def protected_paths(target: Target) -> set[Path]:
    from common import codex_home

    c = codex_home()
    h = home()
    paths = [
        target.skills_dir,
        target.platform_home,
        c,
        c / "agents",
        c / "skills",
        h,
        h / ".cursor",
        h / ".cursor" / "skills",
        h / ".codex",
        h / ".claude",
        h / ".claude" / "skills",
        h / ".config" / "devin",
        h / ".config" / "devin" / "skills",
        h / ".agents",
        h / ".agents" / "skills",
    ]
    if target.project is not None:
        project = target.project
        paths.extend(
            [
                project,
                project / ".cursor",
                project / ".cursor" / "skills",
                project / ".codex",
                project / ".agents",
                project / ".agents" / "skills",
                project / ".claude",
                project / ".claude" / "skills",
                project / ".devin",
                project / ".devin" / "skills",
            ]
        )
    return {p.resolve() for p in paths}


def resolve_allowed(path: Path, target: Target, purpose: str) -> Path:
    resolved = path.expanduser().resolve()
    if resolved in protected_paths(target):
        die(f"{purpose} {display(resolved)} is a protected directory")
    for root in allowed_roots(target):
        root_resolved = root.resolve()
        if resolved.is_relative_to(root_resolved) and resolved != root_resolved:
            return resolved
    die(f"{purpose} {display(resolved)} is outside the install directories for {target.platform} ({target.scope})")


def source_ok(skill: Skill, relative: str, *, glob: bool) -> None:
    if not relative or relative.startswith(("/", "~")):
        die(f"{skill.name}: source must stay inside the skill directory ({relative})")
    parts = Path(relative).parts
    if ".." in parts:
        die(f"{skill.name}: source must not contain .. ({relative})")
    if "**" in relative:
        die(f"{skill.name}: ** globs are not allowed ({relative})")
    has_glob = any("*" in part or "?" in part for part in parts)
    if glob and not has_glob:
        die(f"{skill.name}: expected a glob such as dir/*.toml ({relative})")
    if not glob and has_glob:
        die(f"{skill.name}: unexpected glob in {relative}")


def ignored_file(relative: Path) -> bool:
    if relative.name in {".DS_Store"} or relative.suffix == ".pyc":
        return True
    return any(part in SKIP_DIR_NAMES or part.startswith(".") for part in relative.parts)


def files_under(skill: Skill, src: Path) -> list[Path]:
    if src.is_file():
        return [src]
    if not src.is_dir():
        die(f"{skill.name}: not a file or directory: {src.relative_to(skill.path)}")
    found: list[Path] = []
    for dirpath, dirnames, filenames in os.walk(src):
        dirnames[:] = [name for name in dirnames if name not in SKIP_DIR_NAMES and not name.startswith(".")]
        for name in filenames:
            path = Path(dirpath) / name
            rel = path.relative_to(src)
            if ignored_file(rel) or name.startswith("."):
                continue
            found.append(path)
    if not found:
        die(f"{skill.name}: {src.relative_to(skill.path)} has no files to install")
    return found


def expand_install(skill: Skill, manifest: dict, target: Target) -> list[tuple[Path, Path]]:
    variables = variables_for(target)
    pairs: list[tuple[Path, Path]] = []
    seen: dict[Path, Path] = {}
    where = f"{skill.folder}/{skill.name}"

    def add(src: Path, dest: Path) -> None:
        resolved_src = src.resolve()
        if not resolved_src.is_relative_to(skill.path.resolve()):
            die(f"{skill.name}: source escapes the skill directory ({src})")
        resolved_dest = resolve_allowed(dest, target, f"{skill.name} destination")
        previous = seen.get(resolved_dest)
        if previous is not None and previous != resolved_src:
            die(f"{skill.name}: {display(resolved_dest)} is installed twice")
        seen[resolved_dest] = resolved_src
        pairs.append((resolved_src, resolved_dest))

    for entry in manifest["install"]:
        if isinstance(entry, str):
            source_ok(skill, entry, glob=False)
            src = skill.path / entry
            if not src.exists():
                die(f"{skill.name}: missing {entry}")
            for file in files_under(skill, src):
                rel = file.relative_to(skill.path)
                add(file, target.skill_dir / rel)
            continue
        source = entry["from"]
        dest_text = expand_vars(entry["to"], variables, f"{where} to")
        is_glob = any("*" in part or "?" in part for part in Path(source).parts)
        if is_glob:
            source_ok(skill, source, glob=True)
            matches = [
                path
                for path in sorted(skill.path.glob(source))
                if path.is_file() and not ignored_file(path.relative_to(skill.path))
            ]
            if not matches:
                die(f"{skill.name}: no files matched {source}")
            for file in matches:
                add(file, Path(dest_text) / file.name)
            continue
        source_ok(skill, source, glob=False)
        src = skill.path / source
        if not src.exists():
            die(f"{skill.name}: missing {source}")
        if src.is_dir():
            for file in files_under(skill, src):
                add(file, Path(dest_text) / file.relative_to(src))
            continue
        dest = Path(dest_text)
        if dest_text.endswith("/") or (dest.exists() and dest.is_dir()):
            dest = dest / src.name
        add(src, dest)

    skill_md = target.skill_dir.resolve() / "SKILL.md"
    if not any(dest == skill_md for _, dest in pairs):
        die(f"{skill.name}: manifest must install SKILL.md into the skill directory")
    return pairs


def expand_remove(skill_name: str, manifest: dict, target: Target) -> list[Path]:
    variables = variables_for(target)
    paths: list[Path] = []
    for entry in manifest["remove"]:
        text = expand_vars(entry, variables, f"{skill_name} remove")
        paths.append(resolve_allowed(Path(text), target, f"{skill_name} remove"))
    return paths


def stale_files(skill_dir: Path, keep: set[str]) -> list[Path]:
    if not skill_dir.is_dir() or skill_dir.is_symlink():
        return []
    stale: list[Path] = []
    for dirpath, dirnames, filenames in os.walk(skill_dir):
        dirnames[:] = [name for name in dirnames if name not in SKIP_DIR_NAMES]
        for name in filenames:
            path = Path(dirpath) / name
            rel = path.relative_to(skill_dir).as_posix()
            if rel not in keep:
                stale.append(path.resolve())
    return stale


def empty_dirs(skill_dir: Path, keep: set[str]) -> list[Path]:
    if not skill_dir.is_dir() or skill_dir.is_symlink():
        return []
    doomed: list[Path] = []
    doomed_set: set[Path] = set()
    for dirpath, dirnames, _filenames in os.walk(skill_dir, topdown=False):
        path = Path(dirpath)
        if path == skill_dir:
            continue
        rel_dir = path.relative_to(skill_dir).as_posix()
        keep_under = any(rel.startswith(rel_dir + "/") for rel in keep)
        living_children = [name for name in dirnames if (path / name) not in doomed_set]
        if not keep_under and not living_children:
            doomed.append(path)
            doomed_set.add(path)
    return doomed


def directory_mtime(skill_dir: Path) -> float | None:
    if not skill_dir.is_dir() or skill_dir.is_symlink():
        return None
    times: list[float] = []
    for dirpath, dirnames, filenames in os.walk(skill_dir):
        dirnames[:] = [name for name in dirnames if name not in SKIP_DIR_NAMES and not name.startswith(".")]
        for name in filenames:
            if name == ".DS_Store" or name.startswith("."):
                continue
            times.append((Path(dirpath) / name).stat().st_mtime)
    return max(times) if times else None


def repo_mtime(skill: Skill) -> float:
    manifest = load_manifest(skill)
    times: list[float] = []
    for entry in manifest["install"]:
        if isinstance(entry, str):
            source_ok(skill, entry, glob=False)
            src = skill.path / entry
            if not src.exists():
                die(f"{skill.name}: missing {entry}")
            times.extend(path.stat().st_mtime for path in files_under(skill, src))
            continue
        source = entry["from"]
        is_glob = any("*" in part or "?" in part for part in Path(source).parts)
        if is_glob:
            source_ok(skill, source, glob=True)
            matches = [
                path
                for path in skill.path.glob(source)
                if path.is_file() and not ignored_file(path.relative_to(skill.path))
            ]
            if not matches:
                die(f"{skill.name}: no files matched {source}")
            times.extend(path.stat().st_mtime for path in matches)
            continue
        source_ok(skill, source, glob=False)
        src = skill.path / source
        if not src.exists():
            die(f"{skill.name}: missing {source}")
        times.extend(path.stat().st_mtime for path in files_under(skill, src))
    if not times:
        die(f"{skill.name}: manifest has no files to version")
    return max(times)


def installed_content_mtime(skill: Skill, target: Target) -> float | None:
    pairs = expand_install(skill, load_manifest(skill), target)
    times = [dest.stat().st_mtime for _src, dest in pairs if dest.is_file()]
    return max(times) if times else None


def repo_is_newer(skill: Skill, target: Target) -> bool:
    if not (target.skill_dir / "SKILL.md").is_file():
        return False
    pairs = expand_install(skill, load_manifest(skill), target)
    for src, dest in pairs:
        if not dest.is_file() or src.stat().st_mtime > dest.stat().st_mtime:
            return True
    return False


def parse_frontmatter(path: Path) -> dict[str, str]:
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---\n"):
        return {}
    end = text.find("\n---", 3)
    if end == -1:
        return {}
    meta: dict[str, str] = {}
    current: str | None = None
    for line in text[4:end].splitlines():
        if current and (line.startswith(" ") or line.startswith("\t")):
            extra = line.strip()
            if extra:
                meta[current] = f"{meta[current]} {extra}".strip()
            continue
        if ":" not in line:
            current = None
            continue
        key, value = line.split(":", 1)
        key = key.strip()
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        meta[key] = value
        current = key
    return meta


def plan_install(
    skill: Skill,
    target: Target,
    state: dict,
    skip_keys: set[tuple[str, str, str, str]],
    replace: bool = False,
) -> Plan:
    manifest = load_manifest(skill)
    if target.skill_dir.is_symlink():
        die(f"{display(target.skill_dir)} is a symlink; this tool only manages real directories")
    key = target.key(skill.name)
    current = find_entry(state["installs"], key)
    if target.skill_dir.exists() and current is None and not replace:
        die(
            f"{display(target.skill_dir)} already exists and was not installed by this tool. "
            "Move it aside, then install again."
        )
    if current is not None and Path(current["skillDir"]).resolve() != target.skill_dir.resolve():
        die(f"recorded install path for {skill.name} does not match {display(target.skill_dir)}")

    pairs = expand_install(skill, manifest, target)
    removes = expand_remove(skill.name, manifest, target)
    dests = {dest for _, dest in pairs}
    skill_root = target.skill_dir.resolve()
    for path in removes:
        if path == skill_root or any(dest == path or dest.is_relative_to(path) for dest in dests):
            die(f"{skill.name}: remove path overlaps an installed file: {display(path)}")

    inside: set[str] = set()
    external: list[str] = []
    for dest in dests:
        if dest.is_relative_to(skill_root):
            inside.add(dest.relative_to(skill_root).as_posix())
        else:
            external.append(str(dest))

    actions: list[Action] = []
    for path in removes:
        if not present(path):
            continue
        if still_needed(state["installs"], path, skip_keys):
            actions.append(Action("keep", path, note="still used by another install"))
        else:
            actions.append(Action("remove", path))
    for src, dest in pairs:
        actions.append(Action("copy", dest, src=src))
    if current is not None and not replace:
        for path in stale_files(target.skill_dir, inside):
            if path not in dests:
                actions.append(Action("remove", path))
        for path in empty_dirs(target.skill_dir, inside):
            actions.append(Action("remove", path))
        old_external = [Path(item) for item in current.get("externalFiles", [])]
        for path in old_external:
            if str(path.resolve()) in external:
                continue
            if still_needed(state["installs"], path, skip_keys):
                actions.append(Action("keep", path.resolve(), note="still used by another install"))
            elif present(path):
                actions.append(Action("remove", path.resolve()))
    return Plan(skill, target, actions, external)


def plan_uninstall(
    skill_name: str,
    target: Target,
    manifest: dict | None,
    state: dict,
    skip_keys: set[tuple[str, str, str, str]],
) -> Plan | None:
    current = find_entry(state["installs"], target.key(skill_name))
    if current is None:
        return None
    actions: list[Action] = []
    if manifest is not None:
        for path in expand_remove(skill_name, manifest, target):
            if not present(path):
                continue
            if still_needed(state["installs"], path, skip_keys):
                actions.append(Action("keep", path, note="still used by another install"))
            else:
                actions.append(Action("remove", path))
    for item in current.get("externalFiles", []):
        path = Path(item)
        if still_needed(state["installs"], path, skip_keys):
            actions.append(Action("keep", path, note="still used by another install"))
        elif present(path):
            actions.append(Action("remove", path))
    skill_dir = Path(current["skillDir"])
    if still_needed(state["installs"], skill_dir, skip_keys):
        actions.append(Action("keep", skill_dir, note="still used by another install"))
    elif present(skill_dir):
        actions.append(Action("remove", skill_dir))
    dummy = Skill(skill_name, "", Path("."), ())
    return Plan(dummy, target, actions, [])


def print_plan(heading: str, plan: Plan) -> None:
    from common import _inside

    print(heading)
    skill_root = plan.target.skill_dir.resolve()
    inside: list[str] = []
    outside: list[Action] = []
    for action in plan.actions:
        if action.kind == "copy" and _inside(action.path, skill_root):
            inside.append(action.path.resolve().relative_to(skill_root).as_posix())
        elif action.kind == "copy":
            outside.append(action)
    if inside:
        print(f"  {display(plan.target.skill_dir)}/")
        for rel in inside:
            print(f"    {rel}")
    for action in outside:
        print(f"  copy {display(action.path)}")
    for action in plan.actions:
        if action.kind == "remove":
            print(f"  remove {display(action.path)}")
        elif action.kind == "keep":
            print(f"  keep {display(action.path)} ({action.note})")
    print()


def platforms_for_skill(skill: Skill, platform_filter: list[str] | None, *, strict: bool) -> list[str]:
    if platform_filter is None:
        return list(skill.platforms)
    unsupported = [name for name in platform_filter if name not in skill.platforms]
    if unsupported and strict:
        supported = ", ".join(skill.platforms)
        die(
            f"{skill.name} ({skill.folder}) cannot target {', '.join(unsupported)}; "
            f"it supports {supported}"
        )
    return [name for name in skill.platforms if name in platform_filter]


def require_skill_args(args: argparse.Namespace, command: str) -> None:
    if command == "setup":
        if getattr(args, "skills", None):
            die("setup does not take tool names")
        return
    args.skills = list(dict.fromkeys(args.skills))
    if args.all and args.skills:
        die("pass tool names or --all, not both")
    if command in ("install", "uninstall") and not args.all and not args.skills:
        die("name at least one tool, or pass --all")
    if command in ("list", "installed", "about", "pull", "setup") and args.all:
        die(f"--all does not apply to {command}")
    if command in ("list", "installed", "about", "pull", "setup") and args.dry_run:
        die(f"--dry-run does not apply to {command}")
    if command == "list" and (args.scope_global or args.local or args.directory is not None):
        die("list shows tools in this repository; use installed for installs")
    if command == "about":
        if len(args.skills) != 1:
            die("about, info, and show need one tool name")
        if args.scope_global or args.local or args.directory is not None or args.platform:
            die("about, info, and show list a repository tool and do not take install options")
    if command == "pull" and (
        args.skills or args.scope_global or args.local or args.directory is not None or args.platform
    ):
        die("pull updates this repository and does not take tool or install options")
    if getattr(args, "version", False) and command != "installed":
        die("--version applies to installed")
    if getattr(args, "raw", False) and command not in ("install", "update"):
        die("--raw applies to install and update")


def skills_by_name(skills: list[Skill]) -> dict[str, Skill]:
    return {skill.name: skill for skill in skills}


def resolve_requested(skills: list[Skill], names: list[str], state: dict) -> list[str]:
    known = set(skills_by_name(skills))
    known.update(entry["skill"] for entry in state["installs"] if entry_kind(entry) == "skill" and "skill" in entry)
    missing = [name for name in names if name not in known]
    if missing:
        die("unknown skill: " + ", ".join(missing))
    return names


def resolve_repo_names(skills: list[Skill], names: list[str]) -> None:
    known = set(skills_by_name(skills))
    missing = [name for name in names if name not in known]
    if missing:
        die("unknown skill: " + ", ".join(missing))


def cmd_install(args: argparse.Namespace, skills: list[Skill], state: dict, *, allow_empty: bool = False) -> None:
    from common import save_state

    scopes = selected_scopes(args, ("global",))
    platform_filter = selected_platforms(args.platform)
    project = project_dir(args) if "local" in scopes else None
    if args.directory is not None and "local" not in scopes:
        die("--directory is used with --local")
    catalog = skills_by_name(skills)
    if args.all:
        chosen = []
        for skill in skills:
            platforms = platforms_for_skill(skill, platform_filter, strict=False)
            if platforms:
                chosen.append((skill, platforms))
        if not chosen:
            if allow_empty:
                return
            die("no tools match the selected platforms")
    else:
        resolve_requested(skills, args.skills, state)
        chosen = []
        for name in args.skills:
            skill = catalog.get(name)
            if skill is None:
                die(f"unknown skill: {name}")
            chosen.append((skill, platforms_for_skill(skill, platform_filter, strict=True)))

    plans: list[Plan] = []
    skip_keys: set[tuple[str, str, str, str]] = set()
    targets: list[tuple[Skill, Target]] = []
    for skill, platforms in chosen:
        for scope in scopes:
            for platform in platforms:
                target = make_target(platform, scope, project, skill.name)
                targets.append((skill, target))
                skip_keys.add(target.key(skill.name))
    for skill, target in targets:
        plans.append(plan_install(skill, target, state, skip_keys))

    if args.dry_run:
        print("dry-run: no files will be changed")
    for plan in plans:
        verb = "would install" if args.dry_run else "installed"
        print_plan(f"{verb} {plan.skill.name} → {plan.target.platform} ({plan.target.scope})", plan)
        apply_plan(plan, args.dry_run)
        if not args.dry_run:
            record_skill_install(state, plan)
            save_state(state)


def cmd_uninstall(args: argparse.Namespace, skills: list[Skill], state: dict, *, allow_empty: bool = False) -> None:
    from common import save_state

    scopes = selected_scopes(args, ("global",))
    platform_filter = selected_platforms(args.platform)
    project = project_dir(args) if "local" in scopes else None
    if args.directory is not None and "local" not in scopes:
        die("--directory is used with --local")
    catalog = skills_by_name(skills)

    selected: list[tuple[str, Target, dict | None]] = []
    if args.all:
        for entry in state["installs"]:
            if entry_kind(entry) != "skill":
                continue
            if entry["scope"] not in scopes:
                continue
            if platform_filter is not None and entry["platform"] not in platform_filter:
                continue
            if entry["scope"] == "local" and entry.get("project") != str(project):
                continue
            skill = catalog.get(entry["skill"])
            manifest = load_manifest(skill) if skill else None
            target = make_target(entry["platform"], entry["scope"], project if entry["scope"] == "local" else None, entry["skill"])
            if Path(entry["skillDir"]).resolve() != target.skill_dir.resolve():
                print(
                    f"skipping {entry['skill']} → {entry['platform']} ({entry['scope']}): "
                    f"recorded at {entry['skillDir']}"
                )
                continue
            selected.append((entry["skill"], target, manifest))
    else:
        resolve_requested(skills, args.skills, state)
        for name in args.skills:
            skill = catalog.get(name)
            platforms = list(PLATFORM_ORDER)
            if skill is not None:
                platforms = platforms_for_skill(skill, platform_filter, strict=True)
            elif platform_filter is not None:
                platforms = platform_filter
            manifest = load_manifest(skill) if skill else None
            matched = False
            for scope in scopes:
                for platform in platforms:
                    target = make_target(platform, scope, project, name)
                    if find_entry(state["installs"], target.key(name)):
                        selected.append((name, target, manifest))
                        matched = True
            if not matched:
                where = ", ".join(platforms)
                scope_text = " and ".join(scopes)
                print(f"{name} is not installed for {where} ({scope_text})")

    if args.all and not selected:
        if not allow_empty:
            print("nothing installed for the selected scope")
        return

    skip_keys = {target.key(name) for name, target, _ in selected}
    plans: list[tuple[str, Plan]] = []
    for name, target, manifest in selected:
        plan = plan_uninstall(name, target, manifest, state, skip_keys)
        if plan is not None:
            plans.append((name, plan))

    if args.dry_run and plans:
        print("dry-run: no files will be changed")
    for name, plan in plans:
        verb = "would uninstall" if args.dry_run else "uninstalled"
        print_plan(f"{verb} {name} → {plan.target.platform} ({plan.target.scope})", plan)
        apply_plan(plan, args.dry_run)
        if not args.dry_run:
            drop_install(state, name, plan.target)
            save_state(state)


def cmd_update(args: argparse.Namespace, skills: list[Skill], state: dict, *, allow_empty: bool = False) -> None:
    from common import save_state

    scopes = selected_scopes(args, ("global",))
    platform_filter = selected_platforms(args.platform)
    project = project_dir(args) if "local" in scopes else None
    if args.directory is not None and "local" not in scopes:
        die("--directory is used with --local")
    catalog = skills_by_name(skills)
    if args.skills:
        resolve_repo_names(skills, args.skills)
        chosen = [catalog[name] for name in args.skills]
    else:
        chosen = list(skills)

    outdated: list[tuple[Skill, Target]] = []
    current: list[tuple[Skill, Target]] = []
    installed_names: set[str] = set()
    for skill in chosen:
        strict = bool(args.skills)
        platforms = platforms_for_skill(skill, platform_filter, strict=strict)
        if not platforms:
            continue
        for scope in scopes:
            for platform in platforms:
                target = make_target(platform, scope, project, skill.name)
                if not (target.skill_dir / "SKILL.md").is_file():
                    continue
                installed_names.add(skill.name)
                if repo_is_newer(skill, target):
                    outdated.append((skill, target))
                elif args.skills:
                    current.append((skill, target))

    if args.skills:
        for name in args.skills:
            if name not in installed_names:
                print(f"{name} is not installed")
        for skill, target in current:
            print(f"{skill.name} is up to date for {target.platform} ({target.scope})")
    if not outdated:
        if not args.skills and not allow_empty:
            print("nothing to update")
        return False

    if args.dry_run:
        print("dry-run: no files will be changed")
    for skill, target in outdated:
        manifest = load_manifest(skill)
        key = target.key(skill.name)
        verb = "would update" if args.dry_run else "updated"
        print(f"{verb} {skill.name} → {target.platform} ({target.scope})")
        entry = find_entry(state["installs"], key)
        if entry is not None:
            removal = plan_uninstall(skill.name, target, manifest, state, {key})
            if removal is not None:
                for action in removal.actions:
                    if action.kind == "remove":
                        print(f"  remove {display(action.path)}")
                    elif action.kind == "keep":
                        print(f"  keep {display(action.path)} ({action.note})")
                if not args.dry_run:
                    apply_plan(removal, False)
                    drop_install(state, skill.name, target)
                    save_state(state)
        elif present(target.skill_dir):
            print(f"  remove {display(target.skill_dir)}")
            if not args.dry_run:
                trash_path(target.skill_dir)
        fresh = plan_install(skill, target, state, {key}, replace=True)
        print_plan(f"{'would install' if args.dry_run else 'installed'} {skill.name} → {target.platform} ({target.scope})", fresh)
        if not args.dry_run:
            apply_plan(fresh, False)
            record_skill_install(state, fresh)
            save_state(state)
    return True


def skill_install_status(skill: Skill, state: dict) -> list[tuple[str, str, str]]:
    project = Path.cwd().resolve()
    found: list[tuple[str, str, str]] = []
    for scope in ("global", "local"):
        for platform in skill.platforms:
            target = make_target(platform, scope, project if scope == "local" else None, skill.name)
            if not (target.skill_dir / "SKILL.md").is_file():
                continue
            mark = "▲" if repo_is_newer(skill, target) else "◉"
            found.append((scope, platform, mark))
    return found


def cmd_about(args: argparse.Namespace, skills: list[Skill], state: dict) -> None:
    skill = skills_by_name(skills)[args.skills[0]]
    meta = parse_frontmatter(skill.path / "SKILL.md")
    version = format_version(repo_mtime(skill))
    title = meta.get("name") or skill.name
    fields = [
        ("version", version),
        ("platforms", ", ".join(skill.platforms)),
        ("path", skill.path.relative_to(REPO_ROOT).as_posix()),
    ]
    for key, value in meta.items():
        if key not in ("name", "description") and value:
            fields.append((key, value))
    print_about(
        title,
        "skill",
        fields,
        skill_install_status(skill, state),
        meta.get("description", "").strip(),
    )


def scan_skill_names(skills_dir: Path) -> list[str]:
    if not skills_dir.is_dir():
        return []
    names = []
    for child in sorted(skills_dir.iterdir(), key=lambda p: p.name):
        if child.is_dir() and not child.is_symlink() and (child / "SKILL.md").is_file():
            names.append(child.name)
    return names


def cmd_list(args: argparse.Namespace, skills: list[Skill]) -> None:
    platform_filter = selected_platforms(args.platform)
    name_filter = set(args.skills)
    platforms = platform_filter or list(PLATFORM_ORDER)
    for platform in platforms:
        print(platform)
        names = sorted(
            skill.name
            for skill in skills
            if platform in skill.platforms and (not name_filter or skill.name in name_filter)
        )
        if not names:
            print("  (none)")
        else:
            for name in names:
                print(f"  {name}")
        print()


def cmd_installed(args: argparse.Namespace, skills: list[Skill], state: dict) -> None:
    scopes, project = installed_scopes(args)
    render_installed([("Skills", collect_installed(args, skills, state, scopes, project))], scopes, project)


def collect_installed(
    args: argparse.Namespace,
    skills: list[Skill],
    state: dict,
    scopes: list[str],
    project: Path | None,
) -> list[InstalledGroup]:
    name_filter = set(args.skills)
    repo_names = {skill.name for skill in skills}
    platforms = selected_platforms(args.platform) or list(PLATFORM_ORDER)
    groups: list[InstalledGroup] = []
    for scope in scopes:
        for platform in platforms:
            scope_project = project if scope == "local" else None
            skills_dir, _ = platform_paths(platform, scope, scope_project)
            rows: list[str] = []
            groups.append(InstalledGroup(scope, platform, skills_dir, rows))
            on_disk = scan_skill_names(skills_dir)
            recorded = []
            for entry in state["installs"]:
                if entry_kind(entry) != "skill":
                    continue
                if entry["platform"] != platform or entry["scope"] != scope:
                    continue
                if scope == "local" and entry.get("project") != str(project):
                    continue
                if Path(entry["skillDir"]).resolve().parent != skills_dir.resolve():
                    continue
                recorded.append(entry)
            names = sorted(set(on_disk) | {entry["skill"] for entry in recorded})
            if name_filter:
                names = [name for name in names if name in name_filter]
            if not names:
                continue
            recorded_by_name = {entry["skill"]: entry for entry in recorded}
            catalog = {skill.name: skill for skill in skills}
            show_version = getattr(args, "version", False)
            for name in names:
                entry = recorded_by_name.get(name)
                repo_skill = catalog.get(name)
                target = make_target(platform, scope, scope_project, name)
                newer = repo_skill is not None and repo_is_newer(repo_skill, target)
                notes = []
                if entry is not None:
                    if name not in on_disk:
                        notes.append("not on disk")
                    extra = entry.get("externalFiles") or []
                    if extra:
                        parents = Counter(str(Path(item).parent) for item in extra)
                        bits = []
                        for parent, count in sorted(parents.items()):
                            label = "file" if count == 1 else "files"
                            bits.append(f"{count} {label} in {display(Path(parent))}")
                        notes.append("+ " + ", ".join(bits))
                if newer:
                    mark = "▲"
                elif name in repo_names or entry is not None:
                    mark = "◉"
                else:
                    mark = "◎"
                version = ""
                if show_version:
                    if repo_skill is not None and name in on_disk:
                        mtime = installed_content_mtime(repo_skill, target)
                    elif name in on_disk:
                        mtime = directory_mtime(target.skill_dir)
                    else:
                        mtime = None
                    if mtime is not None:
                        version = f" - {format_version(mtime)}"
                    if newer and repo_skill is not None:
                        repo_version = format_version(repo_mtime(repo_skill))
                        if version:
                            version = f"{version} < {repo_version}"
                        else:
                            version = f" - {repo_version}"
                suffix = f"  ({', '.join(notes)})" if notes else ""
                rows.append(f"{mark} {name}{version}{suffix}")
    return groups
