# Changelog

All notable changes to directory-watcher are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [3.2.1] - 2026-11-05

### Fixed

- A shutdown signal now cancels a running global `onStartWatch` instead of waiting for it to finish on its own.
  - If shutdown interrupts global startup, no target starts watching or runs its startup or finish hooks. Global `onFinishWatch` still runs after the canceled startup has stopped.
- On macOS and Linux, replacing or stopping a hook now waits for the processes in its process group to stop, even if the hook's `sh` exits first.
  - Processes still in the group 20 seconds after SIGTERM are ended with SIGKILL, so descendants that ignore SIGTERM no longer remain after replacement or shutdown.
  - Processes that leave the group, for example with `setsid`, remain outside this cleanup.
- On Windows, canceled hooks are less likely to miss CTRL+BREAK while their programs are still starting.
  - The startup grace period before CTRL+BREAK is now 500 ms instead of 100 ms, and the watcher checks for newly started programs immediately before sending the signal.
  - A program that attaches to the console after the first CTRL+BREAK receives another CTRL+BREAK once 500 ms have passed since it attached, instead of waiting for forced termination after 20 seconds.

## [3.2.0] - 2026-11-05

### Added

- Without `-c`, `--config-path`, the watcher now reads `config.directory-watcher.yml` in the working directory, or `config.yml` if that file does not exist.
  - If neither file exists, the watcher prints `Error: no config file found: tried config.directory-watcher.yml, config.yml (use -c to set a path)` and exits with code 1.
  - If your working directory has both files and you want to keep using `config.yml`, start the watcher with `-c config.yml`.
  - With `-c`, the watcher reads only the given path, as before.

### Changed

- When the watcher reads a config file at startup, it prints a `config: <path>` line on standard output before it checks the settings, so you can tell which file a config error comes from.
  - The line has no log prefix such as `[INFO]`, and `--log-level` does not change it.
  - `-s`, `--silent` turns the line off, and `-n`, `--dry-run` still prints it.
- `--help` now lists `-l`, `--log-level` in a separate `Maintenance Flags:` section. The flag works as before.

## [3.1.0] - 2026-11-05

### Added

- The new `-s`, `--silent` option turns off the hook command lines that the watcher prints before running each command. The output of the hook commands still appears.
- The new `-n`, `--dry-run` option prints the hook command lines without running the commands.
  - The watcher still watches the targets and runs every hook, but skips the commands themselves after building them and checking their working directory, so a missing working directory is reported as in a real run.
  - `-s` and `-n` cannot be used together, and the watcher exits with code 1 before watching starts if both are given.

### Changed

- Before it runs each hook command, the watcher prints a line `<hook name>: <command>` with the same prefix as the hook's output. This applies to the global hooks, the watch target hooks, `onError`, and `onQueueWarning`.
  - In hooks that run on a change, the line shows the command with its template variables filled in, and a command with a `dir` ends with `(dir: <dir>)`.
  - The line has no log prefix such as `[INFO]`, and `--log-level` does not change it. To keep the previous output, start the watcher with `-s`.
- On Windows, the `DEBUG` log level also logs each CTRL+BREAK that the watcher sends to a hook and each exit of a hook process with its exit code and start and exit times.

### Fixed

- On Windows, non-ASCII text, such as Korean, that `cmd` and `powershell` hooks print is no longer garbled in PowerShell and Command Prompt, so you no longer need to run the watcher in Git Bash.
  - While the watcher runs, it sets the console's output code page to UTF-8 (65001), so other programs that share its console, including your shell, also use it until the watcher exits.
  - The watcher restores the original output code page when you stop it with CTRL+C or when it exits with an error.
- On Windows, a CTRL+BREAK meant for a hook that has already exited no longer reaches another process group that reused the hook's process ID.
- The watcher no longer stops with an error when folders change quickly while it watches.
  - A folder that is deleted while the watcher adds it, or the target folder that is deleted again while the watcher adds it back after it was re-created, is treated as deleted.
  - On macOS, re-creating a folder right after it was moved or deleted no longer stops the watcher with `bad file descriptor`, and creating and deleting files repeatedly no longer stops it with a "no such file or directory" error.
  - On Linux, moving a folder and deleting it right after no longer stops the watcher or makes it hang.
- On macOS, a folder created at the path of a folder that was just moved or deleted now reaches the hook as `C`. Before, the event could be missing until the parent folder changed again.
- A folder that is deleted right after it was created is no longer remembered as existing, so a folder created again at the same path reaches the hook as `C`.
- A target folder that is deleted just as watching starts now stops the watcher with exit code 1. Before, the watcher logged that watching had started and watched nothing.
- The `DEBUG` log of the global hooks shows the commands as written in the config instead of internal values.

## [3.0.0] - 2026-11-05

### Added

- The new `option.watch.exclude` list excludes files and folders by path pattern, relative to the target's `path`.
  - Patterns support `*`, `?`, `**`, character classes, and alternatives, and a pattern that starts with `!` watches a path again that an earlier pattern excluded. The last matching pattern decides.
  - A matched folder and everything below it are not added to the watch list, so excluding a large folder such as `node_modules` adds no watches for it.
  - Adding the path a hook writes to `exclude` stops that hook from rerunning itself.
  - A malformed pattern stops the watcher at startup with an error such as `Error: watchTargets[0].option.watch.exclude[1]: invalid pattern "[a"`.
- Hooks receive folder events: a created, moved-in, deleted, or moved-away folder reaches the hook as one `C` or `D` event of the topmost folder.
  - The new variable `{{.IsDir}}` (`DW_IS_DIR`) is `true` for a folder event and `false` for a file event. `{{.ExtName}}` is empty for a folder.
  - The events of the files and subfolders inside such a folder are absorbed into the folder event, so `rm -rf` of a folder with 300 files runs the hook once.
  - The new `option.dirBatch` group sets how long folder events are collected: `waitMillisecond` (default 100, `0` turns collecting off) and `maxWaitMillisecond` (default 1000, or `waitMillisecond` if larger).
  - The new `option.watch.dirEvent` (default `CD`) chooses the folder events that run the hook, and `none` turns them off.
- `option.hookBatch.deliver` chooses the runs made from the events collected in one wait: `last` (default) runs the hook once for the path that changed last, `lastPerPath` runs it once for each changed path, and `each` runs it once for each event.
  - `last` and `lastPerPath` compare the files before the wait with the disk, so a file created and deleted within one wait runs no hook.
- `option.hookBatch.maxWaitMillisecond` (default 1000, or `waitMillisecond` if larger) ends a wait even when events keep arriving.
- `option.queue.size` (default 2048) sets how many runs of a target can wait.
- The new optional global `onQueueWarning` hook runs when the waiting runs of a target reach 70% or 90% of `queue.size` and when they fall back.
  - The hook reads the state from `DW_QUEUE_TARGET`, `DW_QUEUE_LEVEL`, `DW_QUEUE_DIRECTION`, `DW_QUEUE_LENGTH`, `DW_QUEUE_CAPACITY`, and `DW_QUEUE_TOP_PATHS`, which lists the paths with the most waiting runs.
- `option.run.concurrency` (default 1) runs up to that many runs of a target's hooks at the same time.
- `option.run.interrupt` (`none`, `samePath`, or `oldest`; default `none`) chooses the running runs that a new run cancels.
- The new optional global `onError` hook runs once when the watcher stops because events may have been lost: a target's queue is full (`dispatch queue full`) or the OS reports an event overflow.
  - The hook reads the error from `DW_ERROR_TARGET`, `DW_ERROR_KIND`, `DW_ERROR_CAUSE`, `DW_ERROR_MESSAGE`, and `DW_ERROR_POSSIBLE_LOSS`.
  - The hook has a 5-second timeout, and the processes it leaves are ended afterward.
- A failed hook command is logged as an `[ERROR]` line, which the default log level shows, and the line names the target and the command.
  - For a command with template variables, `(with values: ...)` shows the command with the values filled in, so you can see which file it failed on.
- Windows builds are published to a scoop bucket. Install them with `scoop bucket add seungyeop-lee https://github.com/seungyeop-lee/scoop-bucket` and `scoop install seungyeop-lee/directory-watcher`.

### Changed

- Hook commands run in a shell. Every hook command, including `onError`, structured `cmd`, and each entry of a command list, runs as one command line in the shell instead of being split at spaces and run directly.
- The new top-level `shell` setting chooses the shell: `sh`, `cmd`, or `powershell` (Windows PowerShell). Without it, hooks run in `sh` on Unix and in `cmd` on Windows. `cmd` and `powershell` work only on Windows.
- Template variables such as `{{.Path}}` pass file names safely, so a file name never runs as a command. Write them outside quotes, as in `echo 'changed:' {{.Path}}`; each one is then passed as one argument.
- Existing hooks can behave differently: quotes are interpreted instead of printed, so `echo 'done'` prints `done`, and characters such as `$`, `*`, `>`, `|`, and `;` have their shell meaning. On Windows, hooks use `cmd` syntax unless `shell` says otherwise.
- For the details, see [Shell that runs hook commands](docs/guide/configuration.md#shell-that-runs-hook-commands) and [Variables in commands](docs/guide/configuration.md#variables-in-commands) in USAGE.
- The options of a watch target moved into the groups `watch`, `dirBatch`, `hookBatch`, `queue`, and `run`, and a config that still uses a version 2 option stops at startup with an error that names the replacement, such as `Error: watchTargets[0].option.excludeDir is no longer supported; use option.watch.exclude`.

Move the version 2 options as follows:

| Version 2 option | Version 3 option |
|---|---|
| `watchSubDir` | `watch.subDir` |
| `excludeDir`, `excludeSuffix` | `watch.exclude` patterns, such as `'dist/**'` for a folder and `'**/*~'` for a suffix |
| `watchEvent` | `watch.fileEvent` for file events and `watch.dirEvent` for folder events |
| `waitMillisecond` | `hookBatch.waitMillisecond` |
| `noWait: true` | `hookBatch.waitMillisecond: 0` and `hookBatch.deliver: each` |
| `interruptible: true` | `run.interrupt: oldest` |
| `interruptible: false` | `run.interrupt: none` |

- The template variables `{{.FileName}}`, `{{.DirPath}}`, and `{{.DirAbsPath}}` are renamed to `{{.Name}}`, `{{.ParentPath}}`, and `{{.ParentAbsPath}}`, and their environment variables to `DW_NAME`, `DW_PARENT_PATH`, and `DW_PARENT_ABS_PATH`. The old names have no aliases, so replace them in your hooks.
- The watcher checks the commands of `onChange`, `onBeforeChange`, and `onAfterChange` before watching starts, and exits with code 1 if a command uses an unknown variable or anything other than a plain variable, such as `{{if ...}}`, a function, or a pipeline.
  - Write conditions in the shell instead, as in `if [ {{.Event}} = C ]; then echo created; fi` in `sh`.
- A renamed or moved file reaches the hook as `D` for the old path and `C` for the new path, and a file or folder moved out of the target reaches it as `D`.
- A hook that expects the path of a file now also receives folder events. Check `{{.IsDir}}` in the shell, or set `watch.dirEvent: none` on the target.
- Waiting for events ends at most `hookBatch.maxWaitMillisecond` after the first event, so with the defaults a hook runs within 1 second of the first change even while changes keep coming. Before, each new event restarted the wait.
- Runs that wait for a hook are limited by `queue.size`. A run that does not fit stops the watcher with `dispatch queue full` and exit code 1.
- A watch target that cannot start watching stops the watcher with exit code 1: its `path` does not exist, its folder link cannot be resolved, or a folder cannot be added to the watch list. Before, the watcher logged the error and kept running.
  - The `onFinishWatch` of that target does not run, because its `onStartWatch` did not run.
- An event overflow or another error reported by the OS watcher stops the watcher with exit code 1. Before, the error was logged only at the debug level and watching went on.
- A shutdown signal cancels the running hooks, waits until they have stopped, and then runs the finish hooks.
- On Windows, a canceled hook gets CTRL+BREAK instead of being ended at once with `TASKKILL /T /F`, so the hook's programs can clean up before they exit.
  - The programs that the hook starts after the first CTRL+BREAK, such as the next program of a `cmd` command list, get CTRL+BREAK as well.
  - Processes of the hook that are still running 20 seconds after the first CTRL+BREAK are ended together. Replacing or stopping a hook that does not exit on CTRL+BREAK, including some `powershell` hooks, can therefore take up to 20 seconds.
- On Windows, a hook canceled within 100 ms of starting gets CTRL+BREAK only when 100 ms have passed, so a Go program that is still starting does not crash. Replacing or stopping such a hook can take up to 100 ms longer.
- Files and folders whose names start with `.` or `_` are excluded for every event, including updates and deletions of files that already existed. The folder set as `path` is watched regardless of its name.
- If a target's `path` is a link to a folder, the folder it points to is watched, and the variables hold paths based on the resolved folder. Links to folders inside the watched folder are not followed.
- The Homebrew package is now a cask and needs Homebrew 5.1.14 or later. If you installed version 2 with `brew install seungyeop-lee/tap/directory-watcher`, run `brew uninstall --formula directory-watcher` and then `brew install --cask seungyeop-lee/tap/directory-watcher`.
- The Go module path is now `github.com/seungyeop-lee/directory-watcher/v3`, and building it needs Go 1.27 or later. Install it with `go install github.com/seungyeop-lee/directory-watcher/v3@latest`.

### Removed

- The Docker image no longer has the per-architecture tags `<version>-amd64` and `<version>-arm64`. Use `<version>` or `latest`, which cover both `linux/amd64` and `linux/arm64`.

### Fixed

- With `watch.subDir: false`, subfolders that existed when watching started are no longer watched.
- A target whose `path` starts with `.`, such as `../src`, no longer ignores every created file.
- Folders that exist when watching starts and folders created later follow the same watch rules, and excluding `test/dir1` no longer excludes `test/dir10`.
- A target whose `path` is a link to a folder now watches that folder. Before, it logged that watching had started and watched nothing.
- On macOS, events reported for paths outside the target through a folder link no longer run hooks.
- On macOS, files created in a newly watched folder are no longer missed.
- Hook output is no longer cut off: every line a command prints is shown, including the lines printed after it is canceled.
  - If a command leaves a background process that keeps the output open, the watcher stops reading that output 1 second after the command ends and discards what the process prints later.
- A hook whose working directory does not exist now fails with `working directory <directory>: <cause>` instead of an error that reads like a missing executable.
- An error while running no longer prints the CLI usage after the `Error:` line. A wrong flag still prints it.
- The JSON schema now declares draft-07, so validators such as `ajv-cli` can check a config with it.
