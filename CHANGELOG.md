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
- On Windows, a hook canceled within 100 ms of starting gets CTRL+BREAK only when 100 ms have passed, so a Go program that is still starting does not crash. Replacing or stopping such a hook can take up to 100 ms longer.

### Fixed

- On Windows, stopping a hook now stops every process the hook started. Before, a program that a hook started after it got CTRL+BREAK, such as the next program of a `cmd` command list, ran to completion, and so could a Go program that got the signal just as it started. The watcher now sends CTRL+BREAK again to processes that start after the first one, and ends every process of the hook that is still running 20 seconds after it. Replacing or stopping a hook that does not exit on CTRL+BREAK, including some `powershell` hooks, can take up to 20 seconds.
