# aitools

The tool manager for AI coding agents — install, update, and uninstall **skills** and **MCP servers** for Cursor, Codex, Claude Code, and Devin from any repository that follows the [prescribed layout](https://github.com/claudio-silva/ai-tools/blob/main/ABOUT.md).

```sh
npx aitools use @gh/claudio-silva/ai-toolbox
npx aitools list
npx aitools install imagen
npx aitools installed
```

Any `owner/repo` on GitHub, GitLab, or Bitbucket — or a local directory — is a tool source. This package ships the compiled binary; nothing is downloaded at run time.

macOS only for now (arm64 and x64). Full documentation: <https://github.com/claudio-silva/ai-tools>
