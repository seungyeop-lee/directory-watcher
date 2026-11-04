# Directory Watcher usage guide

🇺🇸 English | 🇰🇷 [한국어](USAGE.ko.md)

This guide assumes you have installed Directory Watcher as described in the [README](README.md#installation).

## Command-line options

```shell
Usage:
  directory-watcher [flags]

Flags:
  -c, --config-path string   set config path (default "config.yml")
  -h, --help                 help for directory-watcher
  -l, --log-level string     set log level (default "ERROR")
  -v, --version              version for directory-watcher
```

## Running with Docker

Mount `config.yml` and the directory to watch into the published image, and pass the config path after the image name:

```shell
docker run --rm \
  -v "$PWD/config.yml:/config.yml" \
  -v "$PWD/src:/src" \
  ghcr.io/seungyeop-lee/directory-watcher:latest -c /config.yml
```

- Arguments after the image name are passed to `directory-watcher` as [command-line options](#command-line-options). Without arguments, the image only prints the help and exits.
- The container's working directory is `/`, so relative paths in `config.yml` resolve under `/`. In the example, `path: src` refers to the mounted `/src`. Keep the working directory at `/`: if you change it with `-w`, the container exits with `./directory-watcher: not found`.
- Hooks run inside the container. Mount each directory that a hook's `dir` names at the same path, as in `-v "$PWD/<dir>:/<dir>"`; otherwise the hook's commands fail. `dir: .` is `/` in the container.
- Hook commands can use only the tools in the image. The image is based on Alpine Linux and includes `bash`, `curl`, `git`, `jq`, `make`, and `openssh-client`.

**Known issue: delete events with bind mounts.** When you delete files on the host in a directory that is bind-mounted into a container, the delete event may not always be detected. This is a known issue with the underlying file system notification library (fsnotify). For details, see [fsnotify/fsnotify#592](https://github.com/fsnotify/fsnotify/issues/592).

## config.yml

The `config.yml` file defines the directories to watch and the commands to run for changes in each directory. You can also define commands to run when watching starts or stops. For a practical example, see [`config.example.yml`](config.example.yml). [`directory-watcher.schema.json`](directory-watcher.schema.json) describes the same format as a JSON schema. Running `make run` from the repository root first creates the folders this example watches and then starts the watcher with the example.

```yaml
shell: [ shell that runs hook commands (sh, cmd, powershell), default is sh on Unix and cmd on Windows ]
global:
  lifeCycle:
    onStartWatch: # global onStartWatch hook
      - 'command to execute'
      - dir: 'directory path where the command will be executed'
        cmd: 'command to execute'
      - dir: 'directory path where the command will be executed'
        cmd:
          - 'command to execute 1'
          - 'command to execute 2'
    onBeforeChange: # global onBeforeChange hook
      [ Same specification as global onStartWatch hook ]
    onAfterChange: # global onAfterChange hook
      [ Same specification as global onStartWatch hook ]
    onFinishWatch: # global onFinishWatch hook
      [ Same specification as global onStartWatch hook ]
    onError: # optional global event-loss notification
      [ Same specification as global onStartWatch hook ]
watchTargets:
  - path: [ path of the directory to monitor ]
    lifeCycle:
      onStartWatch: # onStartWatch hook
        [ Same specification as global onStartWatch hook ]
      onChange: # onChange hook
        [ Same specification as global onStartWatch hook ]
      onFinishWatch: # onFinishWatch hook
        [ Same specification as global onStartWatch hook ]
    option:
      exclude:
        - [ path pattern of files and folders to exclude from watching, relative to path ]
      waitMillisecond: [ wait time after an event occurs before executing the hook, default is 100 ]
      watchSubDir: [ whether to monitor subdirectories, default is true ]
      watchEvent: [ monitoring events (C: create, U: update, D: delete), default is "CUD" ]
      noWait: [ whether to execute the hook immediately without waiting after an event occurs, default is false ]
      interruptible: [ whether to stop the currently running command and execute a new command, default is false ]
```

For each change, global `onBeforeChange`, the target's `onChange`, and global `onAfterChange` run in that order. The commands of a hook run one after another. If a command fails, the remaining commands of that run are skipped, including the later hooks of the same change. The failure is logged as an `[ERROR]` line, which the default log level shows:

```text
<target> hook command failed, skipping remaining commands: command <command>: <error>
```

`<target>` is the last element of the target's `path`. The form of `<command>` is in [Shell that runs hook commands](#shell-that-runs-hook-commands). A command stopped because a new change cancelled its run (`interruptible: true`) or because watching stopped is not a failure and is not logged.

If a command of global `onStartWatch` or global `onFinishWatch` fails, the remaining commands of that hook are skipped. The failure is logged as an `[ERROR]` line without the target or hook name:

```text
command <command>: <error>
```

Watching still starts after a global `onStartWatch` failure. A global `onFinishWatch` failure does not change the exit code: it stays 0 after a shutdown signal and 1 after a fatal error.

### Shell that runs hook commands

Every hook command runs in the shell selected by `shell`: global hooks, target hooks, `onError`, structured `cmd`, and each entry of a command list. Quotes, variable expansion, command substitution, `&&`, and redirection follow that shell's rules, and a command that ends with a nonzero exit code is a failure.

| `shell` | How it runs | Available on | Reference that replaces `{{.Path}}` |
|---|---|---|---|
| `sh` | `sh -c <command>` | Every OS. On Windows, `sh` must be on PATH. | `"$DW_PATH"` |
| `cmd` | `cmd.exe /d /v:on /s /c "<command>"` | Windows | `!DW_PATH_ARG!` |
| `powershell` | `powershell.exe -NoProfile -NonInteractive -Command <command>` (Windows PowerShell) | Windows | `${env:DW_PATH_ARG}` |

Without `shell`, hooks run in `sh` on Unix and in `cmd` on Windows. If the value is not in the list, or if `cmd` or `powershell` is selected on Unix, the watcher runs no hook, prints `Error: unknown shell "bash": must be one of sh, cmd, powershell` or `Error: shell "cmd" runs only on Windows`, and exits with code 1.

```yaml
shell: powershell
watchTargets:
  - path: ./src
    lifeCycle:
      onChange:
        - Write-Output "changed: {{.Path}}"
```

A configuration file is meant for one OS, so write its hooks in the syntax of that OS's shell. To use the watcher on several OSes, keep a configuration file per OS or call an OS-specific script from the hook. To run a single hook in another shell, wrap it in the command yourself, as in `bash -c "..."`.

- `cmd` runs with delayed expansion (`/v:on`), so a `!` in a command is read as part of a `!name!` variable reference, and an unpaired `!` is removed.
- Windows PowerShell 5.1 has no `&&`. To run a command only after the previous one succeeded, write `command1; if ($?) { command2 }`.

The `<command>` in a failure log shows how the shell was run together with the command after template variables were replaced with references. For a command with template variables, `(with values: ...)` follows with the command that has the values written in place of the variables, so you can see which file the command failed on. This command is only logged, never run.

```text
command /bin/sh -c cat "$DW_PATH" (with values: cat test/dir2/b.txt): exit status 1
```

### Command execution location

| dir | global onStartWatch, global onFinishWatch, global onError | Other hooks |
|-----|-------------------------------------------|-------------------------|
| O   | Specified dir location                    | Specified dir location  |
| X   | Program execution location                | Specified path location |

Just before a command runs, its working directory is checked. If the working directory does not exist or is not a directory, the command does not run and the remaining commands of that hook are skipped, as for any other failed command. The failure log then carries the working directory in place of `<error>`, in the form below. In the failure log of global `onError`, `onError notification failed: ` is followed by `command <command>: ` and this text.

```text
working directory <working directory>: <cause>
```

### Variables in commands

| Variable        | Environment variable | Description                                             | Example (when `test/dir2` is set as the monitored directory) |
|-----------------|----------------------|---------------------------------------------------------|--------------------------------------------------------------|
| {{.Path}}       | `DW_PATH`            | Relative path of the file where the event occurred      | test/dir2/b.txt                                              |
| {{.AbsPath}}    | `DW_ABS_PATH`        | Absolute path of the file where the event occurred      | /Users/example/directory-watcher/test/dir2/b.txt             |
| {{.FileName}}   | `DW_FILE_NAME`       | File name where the event occurred                      | b.txt                                                        |
| {{.ExtName}}    | `DW_EXT_NAME`        | Extension of the file where the event occurred          | .txt                                                         |
| {{.DirPath}}    | `DW_DIR_PATH`        | Relative path of the directory where the event occurred | test/dir2                                                    |
| {{.DirAbsPath}} | `DW_DIR_ABS_PATH`    | Absolute path of the directory where the event occurred | /Users/example/directory-watcher/test/dir2                   |
| {{.Event}}      | `DW_EVENT`           | Event type (C: create, U: update, D: delete)            | C                                                            |

The watcher puts each value in the environment variable of the table and replaces the variable in the command with a shell reference that reads that environment variable. The reference for each shell is in the table of [Shell that runs hook commands](#shell-that-runs-hook-commands). File names never enter the command string, so a name that contains `;`, `&`, or `$(...)` never runs as a command.

Write variables outside quotes. A variable outside quotes is passed as one argument, even when its value contains spaces. Inside quotes, variables behave as follows:

- In `sh`, inside single quotes as in `'{{.Path}}'`, the literal text `"$DW_PATH"` is passed instead of the value.
- In `sh` and `cmd`, inside double quotes as in `"{{.Path}}"`, the reference ends up outside quotes, so the value can be split into several arguments at spaces.
- `powershell` expands the value inside double quotes as well, as in `"changed: {{.Path}}"`, and keeps the literal text `${env:DW_PATH_ARG}` inside single quotes.

The references of `cmd` and `powershell` read an environment variable whose name ends in `_ARG`, such as `DW_PATH_ARG`. It holds the value changed so that the program reads the original value from its command line, and the original value stays in the environment variable without the suffix, such as `DW_PATH`. When a value that ends in `\`, such as `C:\`, is wrapped in double quotes, the program reads the closing quote as a literal character, so the trailing `\` is doubled.

- `cmd` wraps the value in double quotes and doubles the trailing `\`. Built-in `cmd` commands such as `echo` show these quotes and backslashes as they are.
- `powershell` doubles the trailing `\` only when Windows PowerShell 5.1 wraps the argument in double quotes, that is, when the value contains whitespace. A value with whitespace that ends in `\` then shows one extra `\` at its end in a cmdlet or a string.
- Windows PowerShell 5.1 does not pass an empty string argument to a program. In `powershell`, an empty value outside quotes, such as `{{.ExtName}}` of a file without an extension, leaves the program with one argument fewer.

If `path` is a link to a folder, the folder it points to is watched. The variables above then hold paths based on the fully resolved path instead of `path`. `exclude` patterns exclude the same locations in the linked folder.

## waitMillisecond and noWait

- When an event occurs within `watchTargets.path`, the hook is executed after waiting for waitMillisecond.
- If another event occurs within `watchTargets.path` during the waiting period, the previous event is ignored. That is, the waitMillisecond is reset based on the new event, and the hook is executed after the new waiting period.
- If you want the hook to be executed immediately for every event within `watchTargets.path` without ignoring any events, set noWait to true. (Hooks for events are executed sequentially.)

## Event-loss notification

`global.lifeCycle.onError` is optional and uses the same command list and structured `dir`/`cmd` format as other hooks. It runs at most once per watcher run, only when the first fatal error is an event loss: a target's dispatch queue is full (`dispatch queue full`) or the OS reports an event overflow. Other fatal errors, normal termination, and debounce replacement do not notify.

Error context is passed as environment variables, never interpolated into command templates:

| Environment variable | Value |
|----------------------|-------|
| `DW_ERROR_TARGET` | Watched target associated with the fatal error |
| `DW_ERROR_KIND` | `event_loss` |
| `DW_ERROR_CAUSE` | Original cause string |
| `DW_ERROR_MESSAGE` | Complete fatal error message, in the form `watch target "<target>": <cause>` |
| `DW_ERROR_POSSIBLE_LOSS` | `true` (possible loss, not a lost-event count) |

The notification command also runs in the configured shell, so read the environment variables with that shell's syntax: for example `printf '%s\n' "$DW_ERROR_MESSAGE"` in `sh`, `echo !DW_ERROR_MESSAGE!` in `cmd`, and `Write-Output $env:DW_ERROR_MESSAGE` in `powershell`. In `cmd`, use `!DW_ERROR_MESSAGE!` rather than `%DW_ERROR_MESSAGE%`, which lets a `&` in the value split the command. File-event template variables are not available for this hook. A structured `dir` selects its working directory; without it, the command uses the program execution directory.

After the first fatal error, no new run of normal hooks starts on any target. The notification has an independent, fixed 5-second execution timeout, followed by up to 20 seconds of cancellation cleanup (at most 25 seconds total), regardless of normal-hook cancellation. A notification failure or timeout is logged and never replaces the original fatal error; the CLI still exits nonzero for that original error. Normal finish hooks still run after cancellation, with their existing behavior.

After each notification command completes, fails, or times out, the processes it started that are still running are ended within the same cleanup limit. On Windows, if the watcher cannot prepare a notification command so that these processes can be ended, that command does not run and the failure is logged. Processes started by normal hooks are ended only when the hook is canceled ([Overload and shutdown](#overload-and-shutdown)).

## Overload and shutdown

- Each target keeps at most 256 pending events and runs one run of its hooks at a time. `noWait: true` preserves received-event order; it does not become latest-only under load. Debounce retains the last received valid event, and excluded paths neither execute nor reset its timer. Creation-call order across files does not guarantee notification receipt order, so the last file created is not necessarily the debounce result.
- A full dispatch queue or a watcher error stops processing. The default log includes the watched target and original cause; the CLI cleans up all targets and exits nonzero. Queue saturation is reported as `dispatch queue full`, distinct from an OS-reported overflow. No automatic restart or persistent queue is provided.
- A target that fails to start watching is also a fatal error: its `path` does not exist, its folder link cannot be resolved, or a folder cannot be added to the watch list. Because that target never ran `onStartWatch`, its `onFinishWatch` does not run either. The `onFinishWatch` of the other targets that started watching and global `onFinishWatch` still run, and the CLI exits with code 1.
- With `interruptible: true`, a new run starts only after the canceled previous run has ended. A shutdown signal cancels running start and change commands, discards pending changes, waits for event handling to stop, then runs finish hooks. Finish hooks keep their existing command behavior and have no additional timeout. Targets run independently.
- On Windows, a canceled hook gets CTRL+BREAK, and so do the programs it starts after the cancel, such as the next program of `a & b` in `cmd`. Processes of the hook that are still running 20 seconds after the first CTRL+BREAK are ended together, including processes that ignore the signal or use their own console or process group. Replacement and shutdown wait until every process of the hook has ended, so a hook that does not exit on CTRL+BREAK, including some `powershell` hooks, delays them by up to 20 seconds.
- On Windows, a hook canceled less than 100 ms after it starts gets CTRL+BREAK only when 100 ms have passed since its start, so replacement and shutdown right after a hook starts take up to 100 ms longer.
- On Windows, processes started by a hook that finishes without being canceled keep running.
- What the Windows E2E checks about stopping canceled hooks is listed in [Windows E2E](e2e/docs/windows.md#hook-cancellation-checks).
- Filesystem actions do not map one-to-one to notifications. Linux reports an OS event overflow, which stops processing like a full dispatch queue; macOS does not report one. Application bounds do not guarantee detection of every OS-level loss. See [E2E scenarios and limits](e2e/docs/reliability.md#race-and-overload-verification).

The order of these steps is drawn in the [README diagrams](README.md#how-it-works).

## Default exclusion

- Files and folders whose names start with `.` or `_` are not watched. For example, creating, modifying, or deleting `.env`, or changing anything inside `.git` or `.idea`, does not run hooks. The folder set as `path` is watched regardless of its name.
- Links to folders inside the watched folder are not followed. Changes in a folder outside the watched folder do not run hooks, even when a link inside the watched folder points to it.

## Excluding paths with exclude

`option.exclude` lists path patterns. A file or folder that matches is not watched:

- A matched folder and everything below it are not added to the watch list, whether the folder existed when watching started or was created later. Excluding a large folder such as `node_modules` adds no watches for anything in it.
- Creating, modifying, or deleting a matched file does not run hooks.

```yaml
watchTargets:
  - path: site
    option:
      exclude:
        - '**/node_modules/**'
        - '**/*.log'
        - '!important.log'
```

- Each pattern is matched against the path relative to the target's `path`, with `/` between folders on every OS. A pattern without `**` matches from the top of `path` only: `build.log` matches `site/build.log` but not `site/logs/build.log`.
- `*` matches any part of a name, `?` matches one character, and `**` matches any number of folders. `dist/**` matches the folder `dist` and everything below it, and `**/*.log` matches `.log` files at any depth. Character classes such as `[abc]` and alternatives such as `{js,css}` also work.
- Patterns are checked from top to bottom, and the last pattern that matches decides. A pattern that starts with `!` watches a path again that an earlier pattern excluded. In the example, `important.log` directly under `site` runs hooks, and other `.log` files do not.
- `!` cannot watch a path inside an excluded folder again, because that folder is not watched. To keep part of a folder, exclude the entries inside it instead of the folder: with `dist/*` and `!dist/keep.txt`, only `keep.txt` in `dist` runs hooks. With `dist/**` and `!dist/keep.txt`, `dist` is not watched, so `keep.txt` does not run hooks either.
- `!` also cannot watch a name that [default exclusion](#default-exclusion) excludes.
- Quote each pattern. Unquoted, YAML reads a value that starts with `*` or `!` as its own syntax.

If a pattern is malformed, the watcher runs no hook, prints an error such as `Error: watchTargets[0].option.exclude[1]: invalid pattern "[a"`, and exits with code 1.

### Stopping a hook from rerunning itself

When a hook writes a file inside the target's `path`, that write is a change too, so the hook runs again, writes again, and keeps running. Add the path the hook writes to `exclude`. In this example, `onChange` writes `site/build.log`, and `exclude: ['build.log']` stops that write from running the hook:

```yaml
watchTargets:
  - path: site
    lifeCycle:
      onChange:
        - dir: .
          cmd: make build > site/build.log
    option:
      exclude:
        - 'build.log'
```

With `interruptible: false`, other files changed while the hook runs still run the hook after it finishes.

### Moving from excludeDir and excludeSuffix

Version 3 removed `excludeDir` and `excludeSuffix`. If a target's `option` still has either, the watcher runs no hook, prints the error below, and exits with code 1:

```text
Error: watchTargets[0].option.excludeDir is no longer supported; use option.exclude (see USAGE "Moving from excludeDir and excludeSuffix")
```

Move each value to `exclude`, keeping these differences in mind:

- An `excludeDir` value was a folder path relative to the directory the watcher runs in. An `exclude` pattern is relative to the target's `path`.
- An `excludeSuffix` value was compared with the end of the absolute path, so `out.txt` also excluded `layout.txt`. In `exclude`, write `**/` before the file name to match that name at any depth.
- A pattern moved from `excludeSuffix` that matches a folder name, such as `**/*~` for a folder `backup~`, now also excludes everything in that folder.

For a target with `path: 'site'`:

| Before | After |
|---|---|
| `excludeDir: ['site/dist']` | `exclude: ['dist/**']` |
| `excludeSuffix: ['_test.go']` | `exclude: ['**/*_test.go']` |
| `excludeSuffix: ['~']` | `exclude: ['**/*~']` |
| `excludeSuffix: ['/out.txt']` | `exclude: ['**/out.txt']` |

## Limitations on folder event detection

- By default, hooks are executed based on **file** events within the monitored folder.
- Therefore, hooks are not executed for folder creation or renaming.
- However, hooks are executed for folder deletion (including empty folders) as it cannot be recognized that the target is a folder.
