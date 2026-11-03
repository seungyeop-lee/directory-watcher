# Changelog

All notable changes to directory-watcher are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Hook commands run in a shell. Every hook command, including `onError`, structured `cmd`, and each entry of a command list, runs as one command line in the shell instead of being split at spaces and run directly.
- The new top-level `shell` setting chooses the shell: `sh`, `cmd`, or `powershell` (Windows PowerShell). Without it, hooks run in `sh` on Unix and in `cmd` on Windows. `cmd` and `powershell` work only on Windows.
- Template variables such as `{{.Path}}` pass file names safely, so a file name never runs as a command. Write them outside quotes, as in `echo 'changed:' {{.Path}}`; each one is then passed as one argument.
- Existing hooks can behave differently: quotes are interpreted instead of printed, so `echo 'done'` prints `done`, and characters such as `$`, `*`, `>`, `|`, and `;` have their shell meaning. On Windows, hooks use `cmd` syntax unless `shell` says otherwise.
- For the details, see [Shell that runs hook commands](USAGE.md#shell-that-runs-hook-commands) and [Variables in commands](USAGE.md#variables-in-commands) in USAGE.
