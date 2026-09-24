"""Install, update, uninstall, and list MCP servers from this repository."""

from __future__ import annotations

import argparse
import copy
import getpass
import json
import os
import re
import shutil
import sys
import textwrap
from dataclasses import dataclass
from pathlib import Path

from common import (
    MCP_ROOT,
    PLATFORM_ORDER,
    REPO_ROOT,
    InstalledGroup,
    Target,
    atomic_write,
    codex_home,
    die,
    display,
    drop_install,
    entry_kind,
    find_entry,
    format_version,
    home,
    print_about,
    load_json_object,
    load_state,
    make_target,
    present,
    project_dir,
    save_state,
    selected_platforms,
    selected_scopes,
    trash_path,
)

PLACEHOLDER = re.compile(r"\$([A-Za-z_][A-Za-z0-9_]*)")


@dataclass(frozen=True)
class McpServer:
    name: str
    path: Path
    platforms: tuple[str, ...]
    settings: dict
    message: str
    binary: Path


def discover_mcps() -> list[McpServer]:
    if not MCP_ROOT.is_dir():
        return []
    found: list[McpServer] = []
    names: set[str] = set()
    for folder in sorted(MCP_ROOT.iterdir(), key=lambda p: p.name):
        if not folder.is_dir() or folder.name.startswith("."):
            continue
        manifest_path = folder / "manifest.json"
        if not manifest_path.is_file():
            die(f"{folder.relative_to(REPO_ROOT)} is missing manifest.json")
        data = load_mcp_manifest(folder, manifest_path)
        if folder.name in names:
            die(f"duplicate MCP name {folder.name}")
        names.add(folder.name)
        found.append(data)
    return found


def load_mcp_manifest(folder: Path, path: Path) -> McpServer:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        die(f"{path.relative_to(REPO_ROOT)}: {exc}")
    if not isinstance(data, dict):
        die(f"{path.relative_to(REPO_ROOT)}: manifest must be an object")
    extra = set(data) - {"platforms", "settings", "message"}
    if extra:
        die(f"{path.relative_to(REPO_ROOT)}: unknown keys: {', '.join(sorted(extra))}")
    platforms = data.get("platforms")
    if not isinstance(platforms, list) or not platforms or not all(isinstance(item, str) for item in platforms):
        die(f"{path.relative_to(REPO_ROOT)}: platforms must be a non-empty list of names")
    unknown = [name for name in platforms if name not in PLATFORM_ORDER]
    if unknown:
        die(f"{path.relative_to(REPO_ROOT)}: unknown platform {', '.join(unknown)}")
    settings = data.get("settings")
    if not isinstance(settings, dict) or not settings:
        die(f"{path.relative_to(REPO_ROOT)}: settings must be a non-empty object")
    message = data.get("message")
    if not isinstance(message, str) or not message.strip():
        die(f"{path.relative_to(REPO_ROOT)}: message must be a non-empty string")
    binary = folder / "bin" / folder.name
    if not binary.is_file():
        die(f"{folder.relative_to(REPO_ROOT)} is missing bin/{folder.name}")
    return McpServer(
        folder.name,
        folder,
        tuple(platforms),
        settings,
        message.strip(),
        binary,
    )


def mcp_config_path(platform: str, scope: str, project: Path | None) -> Path:
    if scope == "local":
        if project is None:
            die("local scope needs a project directory")
        if platform == "cursor":
            return project / ".cursor" / "mcp.json"
        if platform == "codex":
            return project / ".codex" / "config.toml"
        if platform == "claude-code":
            return project / ".mcp.json"
        return project / ".devin" / "mcp_config.json"
    if platform == "cursor":
        return home() / ".cursor" / "mcp.json"
    if platform == "codex":
        return codex_home() / "config.toml"
    if platform == "claude-code":
        return home() / ".claude.json"
    return home() / ".config" / "devin" / "mcp_config.json"


def binary_dir() -> Path:
    for directory in (home() / "bin", home() / ".local" / "bin"):
        if directory.is_dir() and os.access(directory, os.W_OK):
            return directory
    die("neither ~/bin nor ~/.local/bin exists and is writable")


def binary_dest(server: McpServer) -> Path:
    return binary_dir() / server.name


def collect_placeholders(value: object) -> list[str]:
    names: list[str] = []
    if isinstance(value, str):
        names.extend(PLACEHOLDER.findall(value))
    elif isinstance(value, dict):
        for item in value.values():
            names.extend(collect_placeholders(item))
    elif isinstance(value, list):
        for item in value:
            names.extend(collect_placeholders(item))
    return list(dict.fromkeys(names))


def replace_placeholders(value: object, mapping: dict[str, str]) -> object:
    if isinstance(value, str):
        def sub(match: re.Match[str]) -> str:
            name = match.group(1)
            if name in mapping:
                return mapping[name]
            return match.group(0)

        return PLACEHOLDER.sub(sub, value)
    if isinstance(value, dict):
        return {key: replace_placeholders(item, mapping) for key, item in value.items()}
    if isinstance(value, list):
        return [replace_placeholders(item, mapping) for item in value]
    return value


def resolve_settings(server: McpServer, command_path: Path, *, raw: bool, dry_run: bool) -> dict:
    mapping = {"BINARY": str(command_path.resolve())}
    names = [name for name in collect_placeholders(server.settings) if name != "BINARY"]
    missing: list[str] = []
    for name in names:
        if raw:
            continue
        if name in os.environ and os.environ[name] != "":
            mapping[name] = os.environ[name]
            continue
        missing.append(name)
    if missing and dry_run:
        print("  would prompt for " + ", ".join("$" + name for name in missing))
    elif missing:
        if not sys.stdin.isatty():
            die(
                f"{server.name}: set {', '.join(missing)} in the environment, "
                "or pass --raw to write placeholders"
            )
        for name in missing:
            value = getpass.getpass(f"  {name}: ")
            if value:
                mapping[name] = value
    return replace_placeholders(copy.deepcopy(server.settings), mapping)


def toml_string(value: str) -> str:
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'


def toml_value(value: object) -> str:
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        return str(value)
    if isinstance(value, list):
        return "[" + ", ".join(toml_value(item) for item in value) + "]"
    return toml_string(str(value))


def settings_to_toml(name: str, settings: dict) -> str:
    lines = [f"[mcp_servers.{name}]"]
    nested: dict[str, dict] = {}
    for key, value in settings.items():
        if isinstance(value, dict):
            nested[key] = value
            continue
        lines.append(f"{key} = {toml_value(value)}")
    for nested_key, nested_value in nested.items():
        lines.append("")
        lines.append(f"[mcp_servers.{name}.{nested_key}]")
        for key, value in nested_value.items():
            lines.append(f"{key} = {toml_value(value)}")
    return "\n".join(lines) + "\n"


def remove_toml_server(text: str, name: str) -> str:
    pattern = re.compile(
        rf"^\[mcp_servers\.{re.escape(name)}(?:\.[^\]]*)?\][^\n]*\n(?:(?!\[).*\n)*",
        re.M,
    )
    cleaned = pattern.sub("", text)
    return re.sub(r"\n{3,}", "\n\n", cleaned).strip() + ("\n" if cleaned.strip() else "")


def toml_server_names(text: str) -> list[str]:
    names = []
    for match in re.finditer(r"^\[mcp_servers\.([A-Za-z0-9_-]+)(?:\.[^\]]+)?\]\s*$", text, re.M):
        name = match.group(1)
        if name not in names:
            names.append(name)
    return names


def toml_command(text: str, name: str) -> str | None:
    match = re.search(
        rf"^\[mcp_servers\.{re.escape(name)}\]\n(?:(?!\[).*\n)*?^command\s*=\s*\"([^\"]*)\"",
        text,
        re.M,
    )
    return match.group(1) if match else None


def json_servers(path: Path) -> dict[str, dict]:
    data = load_json_object(path)
    servers = data.get("mcpServers", {})
    if servers in (None, {}):
        return {}
    if not isinstance(servers, dict):
        die(f"{display(path)}: mcpServers must be an object")
    return servers


def config_has_server(path: Path, name: str) -> bool:
    if not path.exists():
        return False
    if path.suffix == ".toml":
        return name in toml_server_names(path.read_text(encoding="utf-8"))
    return name in json_servers(path)


def config_command(path: Path, name: str) -> str | None:
    if not path.exists():
        return None
    if path.suffix == ".toml":
        return toml_command(path.read_text(encoding="utf-8"), name)
    servers = json_servers(path)
    entry = servers.get(name)
    if not isinstance(entry, dict):
        return None
    command = entry.get("command")
    return command if isinstance(command, str) else None


def write_json_server(path: Path, name: str, settings: dict) -> None:
    data = load_json_object(path)
    servers = data.get("mcpServers")
    if servers is None:
        servers = {}
        data["mcpServers"] = servers
    if not isinstance(servers, dict):
        die(f"{display(path)}: mcpServers must be an object")
    servers[name] = settings
    atomic_write(path, json.dumps(data, indent=2) + "\n")


def remove_json_server(path: Path, name: str) -> None:
    if not path.exists():
        return
    data = load_json_object(path)
    servers = data.get("mcpServers")
    if not isinstance(servers, dict) or name not in servers:
        return
    del servers[name]
    atomic_write(path, json.dumps(data, indent=2) + "\n")


def write_toml_server(path: Path, name: str, settings: dict) -> None:
    text = path.read_text(encoding="utf-8") if path.exists() else ""
    text = remove_toml_server(text, name)
    block = settings_to_toml(name, settings)
    if text and not text.endswith("\n"):
        text += "\n"
    if text and not text.endswith("\n\n"):
        text += "\n"
    atomic_write(path, text + block)


def remove_toml_server_file(path: Path, name: str) -> None:
    if not path.exists():
        return
    atomic_write(path, remove_toml_server(path.read_text(encoding="utf-8"), name))


def write_server(path: Path, name: str, settings: dict) -> None:
    if path.suffix == ".toml":
        write_toml_server(path, name, settings)
    else:
        write_json_server(path, name, settings)


def remove_server(path: Path, name: str) -> None:
    if path.suffix == ".toml":
        remove_toml_server_file(path, name)
    else:
        remove_json_server(path, name)


def copy_binary(server: McpServer, dest: Path, dry_run: bool, verb: str) -> None:
    if dest.resolve() == server.binary.resolve():
        return
    print(f"  {verb} {display(dest)}")
    if dry_run:
        return
    dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(server.binary, dest)
    mode = dest.stat().st_mode | 0o111
    mtime = server.binary.stat().st_mtime
    dest.chmod(mode)
    os.utime(dest, (mtime, mtime))


def copied_to_home_bin(path: Path) -> bool:
    try:
        parent = path.expanduser().resolve().parent
    except OSError:
        return False
    for directory in (home() / "bin", home() / ".local" / "bin"):
        try:
            if parent == directory.resolve():
                return True
        except OSError:
            continue
    return False


def binary_still_needed(state: dict, path: Path, skip_keys: set[tuple[str, str, str, str]]) -> bool:
    try:
        wanted = path.expanduser().resolve()
    except OSError:
        wanted = path.expanduser()
    for entry in state["installs"]:
        if entry_kind(entry) != "mcp":
            continue
        if (entry.get("name"), entry["platform"], entry["scope"], entry.get("project") or "") in skip_keys:
            continue
        stored = entry.get("binaryPath")
        if not stored:
            continue
        try:
            if Path(stored).expanduser().resolve() == wanted:
                return True
        except OSError:
            if stored == str(wanted):
                return True
    return False


def record_mcp_install(state: dict, server: McpServer, target: Target, config: Path, binary: Path) -> None:
    key = target.key(server.name)
    drop_install(state, server.name, target, kind="mcp")
    state["installs"].append(
        {
            "kind": "mcp",
            "name": server.name,
            "platform": target.platform,
            "scope": target.scope,
            "project": target.project_key,
            "configPath": str(config.resolve()),
            "binaryPath": str(binary.resolve()),
        }
    )


def mcps_by_name(servers: list[McpServer]) -> dict[str, McpServer]:
    return {server.name: server for server in servers}


def platforms_for_mcp(server: McpServer, platform_filter: list[str] | None, *, strict: bool) -> list[str]:
    if platform_filter is None:
        return list(server.platforms)
    unsupported = [name for name in platform_filter if name not in server.platforms]
    if unsupported and strict:
        die(
            f"{server.name} cannot target {', '.join(unsupported)}; "
            f"it supports {', '.join(server.platforms)}"
        )
    return [name for name in server.platforms if name in platform_filter]


def resolve_mcp_names(servers: list[McpServer], names: list[str]) -> None:
    known = set(mcps_by_name(servers))
    missing = [name for name in names if name not in known]
    if missing:
        die("unknown MCP: " + ", ".join(missing))


def resolve_mcp_requested(servers: list[McpServer], names: list[str], state: dict) -> None:
    known = set(mcps_by_name(servers))
    known.update(entry["name"] for entry in state["installs"] if entry_kind(entry) == "mcp" and "name" in entry)
    missing = [name for name in names if name not in known]
    if missing:
        die("unknown MCP: " + ", ".join(missing))


def print_message(server: McpServer) -> None:
    print()
    print(textwrap.fill(server.message, width=76))


def cmd_install(args: argparse.Namespace, servers: list[McpServer], state: dict, *, allow_empty: bool = False) -> None:
    scopes = selected_scopes(args, ("global",))
    platform_filter = selected_platforms(args.platform)
    project = project_dir(args) if "local" in scopes else None
    if args.directory is not None and "local" not in scopes:
        die("--directory is used with --local")
    catalog = mcps_by_name(servers)
    raw = getattr(args, "raw", False)
    if args.all:
        chosen = []
        for server in servers:
            platforms = platforms_for_mcp(server, platform_filter, strict=False)
            if platforms:
                chosen.append((server, platforms))
        if not chosen:
            if allow_empty:
                return
            die("no tools match the selected platforms")
    else:
        resolve_mcp_names(servers, args.names)
        chosen = [
            (catalog[name], platforms_for_mcp(catalog[name], platform_filter, strict=True))
            for name in args.names
        ]

    jobs: list[tuple[McpServer, Path, list[tuple[Target, Path]]]] = []
    for server, platforms in chosen:
        dest = binary_dest(server)
        destinations: list[tuple[Target, Path]] = []
        for scope in scopes:
            for platform in platforms:
                target = make_target(platform, scope, project, server.name)
                config = mcp_config_path(platform, scope, project)
                current = find_entry(state["installs"], target.key(server.name), kind="mcp")
                if config_has_server(config, server.name) and current is None:
                    die(
                        f"{server.name} already exists in {display(config)} "
                        "and was not installed by this tool"
                    )
                destinations.append((target, config))
        jobs.append((server, dest, destinations))

    if args.dry_run:
        print("dry-run: no files will be changed")
    for server, dest, destinations in jobs:
        settings = resolve_settings(server, dest, raw=raw, dry_run=args.dry_run)
        verb = "would install" if args.dry_run else "installed"
        copy_binary(server, dest, args.dry_run, verb)
        for target, config in destinations:
            print(f"  {verb} {server.name} → {target.platform} ({target.scope})")
            action = "would edit" if args.dry_run else "edited"
            print(f"  {action} {display(config)}")
            if not args.dry_run:
                write_server(config, server.name, settings)
                record_mcp_install(state, server, target, config, dest)
                save_state(state)
        print_message(server)


def cmd_uninstall(args: argparse.Namespace, servers: list[McpServer], state: dict, *, allow_empty: bool = False) -> None:
    scopes = selected_scopes(args, ("global",))
    platform_filter = selected_platforms(args.platform)
    project = project_dir(args) if "local" in scopes else None
    if args.directory is not None and "local" not in scopes:
        die("--directory is used with --local")
    catalog = mcps_by_name(servers)
    selected: list[tuple[str, Target, Path, Path | None]] = []
    if args.all:
        for entry in state["installs"]:
            if entry_kind(entry) != "mcp":
                continue
            if entry["scope"] not in scopes:
                continue
            if platform_filter is not None and entry["platform"] not in platform_filter:
                continue
            if entry["scope"] == "local" and entry.get("project") != str(project):
                continue
            target = make_target(
                entry["platform"],
                entry["scope"],
                project if entry["scope"] == "local" else None,
                entry["name"],
            )
            binary = Path(entry["binaryPath"]) if entry.get("binaryPath") else None
            selected.append((entry["name"], target, Path(entry["configPath"]), binary))
    else:
        resolve_mcp_requested(servers, args.names, state)
        for name in args.names:
            server = catalog.get(name)
            platforms = list(PLATFORM_ORDER)
            if server is not None:
                platforms = platforms_for_mcp(server, platform_filter, strict=True)
            elif platform_filter is not None:
                platforms = platform_filter
            matched = False
            for scope in scopes:
                for platform in platforms:
                    target = make_target(platform, scope, project, name)
                    entry = find_entry(state["installs"], target.key(name), kind="mcp")
                    if entry is None:
                        continue
                    binary = Path(entry["binaryPath"]) if entry.get("binaryPath") else None
                    selected.append((name, target, Path(entry["configPath"]), binary))
                    matched = True
            if not matched:
                print(f"{name} is not installed")

    if args.all and not selected:
        if not allow_empty:
            print("nothing installed for the selected scope")
        return

    skip_keys = {target.key(name) for name, target, _, _ in selected}
    if args.dry_run:
        print("dry-run: no files will be changed")
    for name, target, config, binary in selected:
        verb = "would uninstall" if args.dry_run else "uninstalled"
        print(f"{verb} {name} → {target.platform} ({target.scope})")
        print(f"  remove {display(config)} {name}")
        if not args.dry_run:
            remove_server(config, name)
        if binary is not None and copied_to_home_bin(binary):
            if binary_still_needed(state, binary, skip_keys):
                print(f"  keep {display(binary)} (still used by another install)")
            elif present(binary):
                print(f"  remove {display(binary)}")
                if not args.dry_run:
                    trash_path(binary)
        if not args.dry_run:
            drop_install(state, name, target, kind="mcp")
            save_state(state)


def mcp_is_newer(server: McpServer, dest: Path, config: Path, entry: dict | None) -> bool:
    if not config_has_server(config, server.name):
        return False
    if dest.is_file() and server.binary.stat().st_mtime > dest.stat().st_mtime:
        return True
    command = config_command(config, server.name)
    expected = dest.resolve()
    if command is not None:
        try:
            if Path(command).expanduser().resolve() != expected:
                return True
        except OSError:
            return True
    if entry is not None and entry.get("binaryPath"):
        try:
            if Path(entry["binaryPath"]).expanduser().resolve() != expected:
                return True
        except OSError:
            return True
    return False


def cmd_update(args: argparse.Namespace, servers: list[McpServer], state: dict, *, allow_empty: bool = False) -> None:
    scopes = selected_scopes(args, ("global",))
    platform_filter = selected_platforms(args.platform)
    project = project_dir(args) if "local" in scopes else None
    if args.directory is not None and "local" not in scopes:
        die("--directory is used with --local")
    catalog = mcps_by_name(servers)
    if args.names:
        resolve_mcp_names(servers, args.names)
        chosen = [catalog[name] for name in args.names]
    else:
        chosen = list(servers)
    raw = getattr(args, "raw", False)

    outdated: list[tuple[McpServer, Target, Path, Path]] = []
    current: list[tuple[McpServer, Target]] = []
    installed_names: set[str] = set()
    for server in chosen:
        platforms = platforms_for_mcp(server, platform_filter, strict=bool(args.names))
        if not platforms:
            continue
        dest = binary_dest(server)
        for scope in scopes:
            for platform in platforms:
                target = make_target(platform, scope, project, server.name)
                config = mcp_config_path(platform, scope, project)
                if not config_has_server(config, server.name):
                    continue
                installed_names.add(server.name)
                entry = find_entry(state["installs"], target.key(server.name), kind="mcp")
                if mcp_is_newer(server, dest, config, entry):
                    outdated.append((server, target, config, dest))
                elif args.names:
                    current.append((server, target))

    if args.names:
        for name in args.names:
            if name not in installed_names:
                print(f"{name} is not installed")
        for server, target in current:
            print(f"{server.name} is up to date for {target.platform} ({target.scope})")
    if not outdated:
        if not args.names and not allow_empty:
            print("nothing to update")
        return False

    if args.dry_run:
        print("dry-run: no files will be changed")
    settings_by_name: dict[str, dict] = {}
    copied: set[str] = set()
    messaged: set[str] = set()
    for server, target, config, dest in outdated:
        verb = "would update" if args.dry_run else "updated"
        print(f"  {verb} {server.name} → {target.platform} ({target.scope})")
        print(f"    remove {display(config)} {server.name}")
        if not args.dry_run:
            remove_server(config, server.name)
            drop_install(state, server.name, target, kind="mcp")
            save_state(state)
        if server.name not in copied:
            copy_binary(server, dest, args.dry_run, verb)
            copied.add(server.name)
        if server.name not in settings_by_name:
            settings_by_name[server.name] = resolve_settings(
                server, dest, raw=raw, dry_run=args.dry_run
            )
        if not args.dry_run:
            write_server(config, server.name, settings_by_name[server.name])
            record_mcp_install(state, server, target, config, dest)
            save_state(state)
        if server.name not in messaged:
            print_message(server)
            messaged.add(server.name)
    return True


def mcp_install_status(server: McpServer, state: dict) -> list[tuple[str, str, str]]:
    project = Path.cwd().resolve()
    dest = binary_dest(server)
    found: list[tuple[str, str, str]] = []
    for scope in ("global", "local"):
        for platform in server.platforms:
            scope_project = project if scope == "local" else None
            config = mcp_config_path(platform, scope, scope_project)
            if not config_has_server(config, server.name):
                continue
            target = make_target(platform, scope, scope_project, server.name)
            entry = find_entry(state["installs"], target.key(server.name), kind="mcp")
            mark = "▲" if mcp_is_newer(server, dest, config, entry) else "◉"
            found.append((scope, platform, mark))
    return found


def cmd_about(args: argparse.Namespace, servers: list[McpServer], state: dict) -> None:
    server = mcps_by_name(servers)[args.names[0]]
    version = format_version(server.binary.stat().st_mtime)
    fields = [
        ("version", version),
        ("platforms", ", ".join(server.platforms)),
        ("path", server.path.relative_to(REPO_ROOT).as_posix()),
        ("binary", server.binary.relative_to(REPO_ROOT).as_posix()),
    ]
    print_about(server.name, "MCP server", fields, mcp_install_status(server, state), server.message)


def collect_installed(
    args: argparse.Namespace,
    servers: list[McpServer],
    state: dict,
    scopes: list[str],
    project: Path | None,
) -> list[InstalledGroup]:
    name_filter = set(args.names)
    catalog = mcps_by_name(servers)
    repo_names = set(catalog)
    platforms = selected_platforms(args.platform) or list(PLATFORM_ORDER)
    show_version = getattr(args, "version", False)
    groups: list[InstalledGroup] = []
    for scope in scopes:
        for platform in platforms:
            scope_project = project if scope == "local" else None
            config = mcp_config_path(platform, scope, scope_project)
            rows: list[str] = []
            groups.append(InstalledGroup(scope, platform, config, rows))
            on_disk: list[str] = []
            if config.exists():
                if config.suffix == ".toml":
                    on_disk = toml_server_names(config.read_text(encoding="utf-8"))
                else:
                    on_disk = sorted(json_servers(config))
            recorded = []
            for entry in state["installs"]:
                if entry_kind(entry) != "mcp":
                    continue
                if entry["platform"] != platform or entry["scope"] != scope:
                    continue
                if scope == "local" and entry.get("project") != str(project):
                    continue
                recorded.append(entry)
            names = sorted(set(on_disk) | {entry["name"] for entry in recorded})
            if name_filter:
                names = [name for name in names if name in name_filter]
            if not names:
                continue
            recorded_by_name = {entry["name"]: entry for entry in recorded}
            for name in names:
                entry = recorded_by_name.get(name)
                repo = catalog.get(name)
                dest = binary_dest(repo) if repo is not None else None
                newer = False
                if repo is not None and dest is not None:
                    newer = mcp_is_newer(repo, dest, config, entry)
                notes = []
                if entry is not None and name not in on_disk:
                    notes.append("not in config")
                if newer:
                    mark = "▲"
                elif name in repo_names or entry is not None:
                    mark = "◉"
                else:
                    mark = "◎"
                version = ""
                if show_version:
                    binary = None
                    if entry and entry.get("binaryPath") and Path(entry["binaryPath"]).is_file():
                        binary = Path(entry["binaryPath"])
                    elif dest is not None and dest.is_file():
                        binary = dest
                    if binary is not None:
                        version = f" - {format_version(binary.stat().st_mtime)}"
                    if newer and repo is not None:
                        repo_version = format_version(repo.binary.stat().st_mtime)
                        if version:
                            version = f"{version} < {repo_version}"
                        else:
                            version = f" - {repo_version}"
                suffix = f"  ({', '.join(notes)})" if notes else ""
                rows.append(f"{mark} {name}{version}{suffix}")
    return groups
