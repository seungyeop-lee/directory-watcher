# Directory Watcher usage guide

🇺🇸 English | 🇰🇷 [한국어](USAGE.ko.md)

- This guide assumes you have installed Directory Watcher as described in the [README](README.md#installation).

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

- **Known issue: delete events with bind mounts.** When you delete files on the host in a directory that is bind-mounted into a container, the delete event may not always be detected.
  - This is a known issue with the underlying file system notification library (fsnotify).
  - For details, see [fsnotify/fsnotify#592](https://github.com/fsnotify/fsnotify/issues/592).

## config.yml

- The `config.yml` file defines the directories to watch and the commands to run for changes in each directory.
- You can also define commands to run when watching starts or stops.
- For a practical example, see [`config.example.yml`](config.example.yml).
- [`directory-watcher.schema.json`](directory-watcher.schema.json) describes the same format as a JSON schema.
- Running `make run` from the repository root first creates the folders this example watches and then starts the watcher with the example.

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
    onQueueWarning: # optional global warning that a target's queue is filling up
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
      watch: # what to watch
        subDir: [ whether to monitor subdirectories, default is true ]
        exclude:
          - [ path pattern of files and folders to exclude from watching, relative to path ]
        fileEvent: [ file events that run the hook (C: create or move in, U: update, D: delete or move away), or none, default is "CUD" ]
        dirEvent: [ folder events that run the hook (C: create or move in, D: delete or move away), or none, default is "CD" ]
      dirBatch: # collecting events into folder events
        waitMillisecond: [ quiet time after the last event before collected folder events are sent, 0 turns collecting off, default is 100 ]
        maxWaitMillisecond: [ longest time folder events are collected, not less than waitMillisecond, default is 1000 or waitMillisecond if larger ]
      hookBatch: # waiting for events and merging them into runs of the hook
        waitMillisecond: [ quiet time after the last event before the collected events become runs, 0 does not wait, default is 100 ]
        maxWaitMillisecond: [ longest time events are collected, not less than waitMillisecond, default is 1000 or waitMillisecond if larger ]
        deliver: [ runs made from the collected events (last, lastPerPath, each), default is last ]
      queue: # runs waiting for the hook
        size: [ most runs that can wait, at least 1, default is 2048 ]
      run: # running the hook
        concurrency: [ most runs of the hook at the same time, at least 1, default is 1 ]
        interrupt: [ running runs that a new run cancels (none, samePath, oldest), default is none ]
```

- For each change, global `onBeforeChange`, the target's `onChange`, and global `onAfterChange` run in that order.
- The commands of a hook run one after another.
- If a command fails, the remaining commands of that run are skipped, including the later hooks of the same change.

The failure is logged as an `[ERROR]` line, which the default log level shows:

```text
<target> hook command failed, skipping remaining commands: command <command>: <error>
```

- `<target>` is the last element of the target's `path`.
- The form of `<command>` is in [Shell that runs hook commands](#shell-that-runs-hook-commands).
- A command stopped because a new run cancelled its run (`run.interrupt`) or because watching stopped is not a failure and is not logged.

- If a command of global `onStartWatch` or global `onFinishWatch` fails, the remaining commands of that hook are skipped.

The failure is logged as an `[ERROR]` line without the target or hook name:

```text
command <command>: <error>
```

- Watching still starts after a global `onStartWatch` failure.
- A global `onFinishWatch` failure does not change the exit code: it stays 0 after a shutdown signal and 1 after a fatal error.

### Shell that runs hook commands

- Every hook command runs in the shell selected by `shell`: global hooks, target hooks, `onError`, `onQueueWarning`, structured `cmd`, and each entry of a command list.
- Quotes, variable expansion, command substitution, `&&`, and redirection follow that shell's rules, and a command that ends with a nonzero exit code is a failure.

| `shell` | How it runs | Available on | Reference that replaces `{{.Path}}` |
|---|---|---|---|
| `sh` | `sh -c <command>` | Every OS. On Windows, `sh` must be on PATH. | `"$DW_PATH"` |
| `cmd` | `cmd.exe /d /v:on /s /c "<command>"` | Windows | `!DW_PATH_ARG!` |
| `powershell` | `powershell.exe -NoProfile -NonInteractive -Command <command>` (Windows PowerShell) | Windows | `${env:DW_PATH_ARG}` |

- Without `shell`, hooks run in `sh` on Unix and in `cmd` on Windows.
- If the value is not in the list, or if `cmd` or `powershell` is selected on Unix, the watcher runs no hook, prints `Error: unknown shell "bash": must be one of sh, cmd, powershell` or `Error: shell "cmd" runs only on Windows`, and exits with code 1.

```yaml
shell: powershell
watchTargets:
  - path: ./src
    lifeCycle:
      onChange:
        - Write-Output "changed: {{.Path}}"
```

- A configuration file is meant for one OS, so write its hooks in the syntax of that OS's shell.
- To use the watcher on several OSes, keep a configuration file per OS or call an OS-specific script from the hook.
- To run a single hook in another shell, wrap it in the command yourself, as in `bash -c "..."`.

- `cmd` runs with delayed expansion (`/v:on`), so a `!` in a command is read as part of a `!name!` variable reference, and an unpaired `!` is removed.
- Windows PowerShell 5.1 has no `&&`. To run a command only after the previous one succeeded, write `command1; if ($?) { command2 }`.

- The `<command>` in a failure log shows how the shell was run together with the command after template variables were replaced with references.
  - For a command with template variables, `(with values: ...)` follows with the command that has the values written in place of the variables, so you can see which file the command failed on.
  - This command is only logged, never run.

```text
command /bin/sh -c cat "$DW_PATH" (with values: cat test/dir2/b.txt): exit status 1
```

### Command execution location

| dir | global onStartWatch, global onFinishWatch, global onError, global onQueueWarning | Other hooks |
|-----|-------------------------------------------|-------------------------|
| O   | Specified dir location                    | Specified dir location  |
| X   | Program execution location                | Specified path location |

- Just before a command runs, its working directory is checked.
- If the working directory does not exist or is not a directory, the command does not run and the remaining commands of that hook are skipped, as for any other failed command.
- The failure log then carries the working directory in place of `<error>`, in the form below.
  - In the failure log of global `onError` and global `onQueueWarning`, `onError notification failed: ` or `onQueueWarning notification failed: ` is followed by `command <command>: ` and this text.

```text
working directory <working directory>: <cause>
```

### Variables in commands

| Variable           | Environment variable | Description                                                                         | Example (when `test/dir2` is set as the monitored directory) |
|--------------------|----------------------|-------------------------------------------------------------------------------------|--------------------------------------------------------------|
| {{.Path}}          | `DW_PATH`            | Relative path of the file or folder where the event occurred                        | test/dir2/b.txt                                              |
| {{.AbsPath}}       | `DW_ABS_PATH`        | Absolute path of the file or folder where the event occurred                        | /Users/example/directory-watcher/test/dir2/b.txt             |
| {{.Name}}          | `DW_NAME`            | Name of the file or folder where the event occurred                                 | b.txt                                                        |
| {{.ExtName}}       | `DW_EXT_NAME`        | Extension of the file where the event occurred; empty for a folder                  | .txt                                                         |
| {{.ParentPath}}    | `DW_PARENT_PATH`     | Relative path of the folder that holds the file or folder where the event occurred  | test/dir2                                                    |
| {{.ParentAbsPath}} | `DW_PARENT_ABS_PATH` | Absolute path of the folder that holds the file or folder where the event occurred  | /Users/example/directory-watcher/test/dir2                   |
| {{.Event}}         | `DW_EVENT`           | Event type (C: create or move in, U: update, D: delete or move away)               | C                                                            |
| {{.IsDir}}         | `DW_IS_DIR`          | `true` for a [folder event](#folder-events), `false` for a file event               | false                                                        |

- Only the hooks that run for a change use these variables: the target's `onChange` and global `onBeforeChange` and `onAfterChange`.
- The other hooks run their commands as written, so `{{.Path}}` in them stays as that literal text.

- Before watching starts, the watcher checks the commands of the hooks that run for a change.

If a command uses a variable that is not in the table, the watcher runs no hook, prints an error like the one below, and exits with code 1:

```text
Error: watchTargets[0].lifeCycle.onChange[1]: unknown template variable "FilePath"; available: Path, AbsPath, Name, ExtName, ParentPath, ParentAbsPath, Event, IsDir
```

- The location before the colon points to the command in the config.
  - `onChange[1]` is the second item of `onChange`, and `onChange[0].cmd[1]` is the second command in the `cmd` list of the first item.
  - A global hook starts with `global.lifeCycle.`, as in `global.lifeCycle.onBeforeChange[0]`.
- A command whose template fails for every event, such as `{{.Path.Missing}}`, also stops the watcher, and its error has `can't execute command:` after the location.
  - Every variable holds a string, so reading a field of a variable, as `.Missing` does, always fails.

- The watcher puts each value in the environment variable of the table and replaces the variable in the command with a shell reference that reads that environment variable.
- The reference for each shell is in the table of [Shell that runs hook commands](#shell-that-runs-hook-commands).
- File names never enter the command string, so a name that contains `;`, `&`, or `$(...)` never runs as a command.

- Write variables outside quotes.
- A variable outside quotes is passed as one argument, even when its value contains spaces.

Inside quotes, variables behave as follows:

- In `sh`, inside single quotes as in `'{{.Path}}'`, the literal text `"$DW_PATH"` is passed instead of the value.
- In `sh` and `cmd`, inside double quotes as in `"{{.Path}}"`, the reference ends up outside quotes, so the value can be split into several arguments at spaces.
- `powershell` expands the value inside double quotes as well, as in `"changed: {{.Path}}"`, and keeps the literal text `${env:DW_PATH_ARG}` inside single quotes.

- The references of `cmd` and `powershell` read an environment variable whose name ends in `_ARG`, such as `DW_PATH_ARG`.
  - That variable holds the value changed so that the program reads the original value from its command line, and the original value stays in the environment variable without the suffix, such as `DW_PATH`.
- When a value that ends in `\`, such as `C:\`, is wrapped in double quotes, the program reads the closing quote as a literal character, so the trailing `\` is doubled.

- `cmd` wraps the value in double quotes and doubles the trailing `\`. Built-in `cmd` commands such as `echo` show these quotes and backslashes as they are.
- `powershell` doubles the trailing `\` only when Windows PowerShell 5.1 wraps the argument in double quotes, that is, when the value contains whitespace. A value with whitespace that ends in `\` then shows one extra `\` at its end in a cmdlet or a string.
- Windows PowerShell 5.1 does not pass an empty string argument to a program. In `powershell`, an empty value outside quotes, such as `{{.ExtName}}` of a file without an extension, leaves the program with one argument fewer.

- If `path` is a link to a folder, the folder it points to is watched.
  - The variables above then hold paths based on the fully resolved path instead of `path`.
  - `exclude` patterns exclude the same locations in the linked folder.

### Moving from FileName, DirPath, and DirAbsPath

- Version 3 renamed three template variables and their environment variables.
  - The values of the renamed variables stay the same.
- A hook that still uses an old template variable stops the watcher at startup with the `unknown template variable` error in [Variables in commands](#variables-in-commands).
- The watcher no longer sets the old environment variables and does not check commands for them.
  - A command or script that reads `DW_FILE_NAME`, `DW_DIR_PATH`, or `DW_DIR_ABS_PATH` runs without the value, so search your hook commands and the scripts they run for these names.

Rename each variable as follows:

| Before | After |
|---|---|
| `{{.FileName}}`, `DW_FILE_NAME` | `{{.Name}}`, `DW_NAME` |
| `{{.DirPath}}`, `DW_DIR_PATH` | `{{.ParentPath}}`, `DW_PARENT_PATH` |
| `{{.DirAbsPath}}`, `DW_DIR_ABS_PATH` | `{{.ParentAbsPath}}`, `DW_PARENT_ABS_PATH` |

### Moving from version 2 options

- Version 3 groups the target options under `watch`, `dirBatch`, `hookBatch`, `queue`, and `run` in `option`.

If a target's `option` still has `watchSubDir`, `waitMillisecond`, `noWait`, or `interruptible` directly under it, the watcher runs no hook, prints an error like the one below, and exits with code 1:

```text
Error: watchTargets[0].option.watchSubDir is no longer supported; use option.watch.subDir (see USAGE "Moving from version 2 options")
```

Move each version 2 option as follows:

| Version 2 | Version 3 |
|---|---|
| `watchSubDir: false` | `watch: {subDir: false}` |
| `waitMillisecond: 500` | `hookBatch: {waitMillisecond: 500}` |
| `waitMillisecond: 0` | No `waitMillisecond`: in version 3, `0` means no waiting instead of the default 100 |
| `waitMillisecond` above 1000, such as `waitMillisecond: 3000` | `hookBatch: {waitMillisecond: 3000}`: without `maxWaitMillisecond`, the longest wait is the same as `waitMillisecond` instead of 1000 |
| `noWait: true` | `hookBatch: {waitMillisecond: 0, deliver: each}` |
| `noWait: false` | No `noWait` |
| `excludeDir`, `excludeSuffix` | `watch.exclude`, as described in [Moving from excludeDir and excludeSuffix](#moving-from-excludedir-and-excludesuffix) |
| `watchEvent` | `watch.fileEvent` and `watch.dirEvent`, as described in [Moving from watchEvent](#moving-from-watchevent) |
| `interruptible: true` | `run: {interrupt: oldest}` |
| `interruptible: false` | No `interruptible`: `run.interrupt` defaults to `none` |

- With `noWait: false`, version 2 ran the hook once with the last event received in the wait. Version 3 defaults to `deliver: last`, which runs the hook with the final change of the path that changed last, as described in [Waiting and merging with hookBatch](#waiting-and-merging-with-hookbatch).
  - A file created and deleted within one wait ran the hook with `D` in version 2 and runs no hook in version 3.
  - A file whose create event the watcher missed reaches the hook as `C` instead of `U`.
- Version 2 kept waiting while events kept arriving. Version 3 runs the hook at most `hookBatch.maxWaitMillisecond` (default 1000, or `hookBatch.waitMillisecond` if larger) after the first event.
- Version 2 left events that arrived while a hook ran waiting in the OS. Version 3 keeps reading them and holds at most `queue.size` (default 2048) runs per target waiting for the hook. A run that does not fit stops the watcher, as described in [Queue size and warnings](#queue-size-and-warnings).

## Renamed and moved files

- The watcher reports a renamed or moved file as a deletion (`D`) of its old path and a creation (`C`) of its new path inside the target.

The hook receives these events for each action:

| Action | Events the hook receives |
|---|---|
| Rename in the same folder | `D` for the old name, then `C` for the new name |
| Move to another folder in the target | `D` for the old path, then `C` for the new path |
| Move out of the target | `D` for the old path |
| Move into the target | `C` for the new path |

- With `hookBatch.deliver: last`, the default, only the path that changed last within one wait runs the hook, so a rename or a move inside the target runs the hook once with `C` for the new path.
- If the old path matches `exclude` or the [default exclusion](#default-exclusion), the hook does not receive the `D`.
- macOS can report the new path before the old one. The watcher then holds the `C` and the events after it until the `D` for the old path arrives, for at most 1 second.
- Renamed and moved folders are covered in [Folder events](#folder-events).

### Moving from version 2 rename events

- Before version 3, on macOS and Linux, the hook received only `C` for the new path of a file renamed or moved inside the target, and no event for a file moved out of the target.
  - On Windows, a move to another folder or out of the target already arrived as `D` for the old path, but a rename in the same folder did not.
- A hook that runs on `D` now also runs when a file is renamed or moved away, and it cannot tell that case from a deletion.

## Waiting and merging with hookBatch

- The watcher collects the events of a target and turns them into runs of the hook once no event has arrived for `hookBatch.waitMillisecond` (default 100), or `hookBatch.maxWaitMillisecond` (default 1000, or `hookBatch.waitMillisecond` if larger) after the first event.
- Collecting goes on while a hook runs. Each wait that ends puts its runs in the target's queue, and runs from different waits are never merged.
- With `hookBatch.waitMillisecond: 0`, the watcher does not wait: each event becomes runs on its own, and `maxWaitMillisecond` is not used.

If a written `maxWaitMillisecond` is less than `waitMillisecond`, or `deliver` is not one of the values below, the watcher runs no hook, prints an error such as the one below, and exits with code 1:

```text
Error: watchTargets[0].option.hookBatch: maxWaitMillisecond 1000 is less than waitMillisecond 3000
```

`hookBatch.deliver` chooses the runs made from the events collected in one wait:

| `deliver` | Runs |
|---|---|
| `last` (default) | One run for the path that changed last, among the paths with a final change. No run if no path has one. |
| `lastPerPath` | One run for each path with a final change, in the order the paths last changed. |
| `each` | One run for each event, in the order the events happened. Final changes are not computed. |

For example, if one wait collects the creation of `a.txt`, an update of `a.txt`, an update of `b.txt`, the creation of `x.swp`, and the deletion of `x.swp`:

- `last` runs the hook once, with `U` for `b.txt`.
- `lastPerPath` runs the hook with `C` for `a.txt`, then with `U` for `b.txt`.
- `each` runs the hook five times.

`last` and `lastPerPath` find the final change of each path by comparing the watcher's record of the target's files before the wait with the disk right before the runs are made:

| Before the wait | Now | Final change |
|---|---|---|
| Absent | Present | `C` |
| Present | Present | `U`, or no run for a folder |
| Present | Absent | `D` |
| Absent | Absent | No run |

- A file whose create event the watcher missed is absent from the record, so its first change reaches the hook as `C`.
- `watch.fileEvent` and `watch.dirEvent` choose among the final changes, or among the events with `each`.
- A path can change again after the watcher checks the disk and before the hook runs. That change follows as a new event, so the hook has to read the actual file when it runs.
- `lastPerPath` runs paths in the order they last changed. Use `each` when the order between files matters.

## Event-loss notification

- `global.lifeCycle.onError` is optional and uses the same command list and structured `dir`/`cmd` format as other hooks.
- It runs at most once per watcher run, only when the first fatal error is an event loss: a target's dispatch queue is full (`dispatch queue full`) or the OS reports an event overflow.
- Other fatal errors, normal termination, and events merged by `hookBatch` do not notify.

Error context is passed as environment variables, never interpolated into command templates:

| Environment variable | Value |
|----------------------|-------|
| `DW_ERROR_TARGET` | Watched target associated with the fatal error |
| `DW_ERROR_KIND` | `event_loss` |
| `DW_ERROR_CAUSE` | Original cause string |
| `DW_ERROR_MESSAGE` | Complete fatal error message, in the form `watch target "<target>": <cause>` |
| `DW_ERROR_POSSIBLE_LOSS` | `true` (possible loss, not a lost-event count) |

- The notification command also runs in the configured shell, so read the environment variables with that shell's syntax: for example `printf '%s\n' "$DW_ERROR_MESSAGE"` in `sh`, `echo !DW_ERROR_MESSAGE!` in `cmd`, and `Write-Output $env:DW_ERROR_MESSAGE` in `powershell`.
  - In `cmd`, use `!DW_ERROR_MESSAGE!` rather than `%DW_ERROR_MESSAGE%`, which lets a `&` in the value split the command.
- File-event template variables are not available for this hook.
- A structured `dir` selects its working directory; without it, the command uses the program execution directory.

- After the first fatal error, no new run of normal hooks starts on any target.
- The notification has an independent, fixed 5-second execution timeout, followed by up to 20 seconds of cancellation cleanup (at most 25 seconds total), regardless of normal-hook cancellation.
- A notification failure or timeout is logged and never replaces the original fatal error; the CLI still exits nonzero for that original error.
- Normal finish hooks still run after cancellation, with their existing behavior.

- After each notification command completes, fails, or times out, the processes it started that are still running are ended within the same cleanup limit.
  - On Windows, if the watcher cannot prepare a notification command so that these processes can be ended, that command does not run and the failure is logged.
- Processes started by normal hooks are ended only when the hook is canceled ([Overload and shutdown](#overload-and-shutdown)).

## Queue size and warnings

- The runs of a target wait in the target's queue until they start, as described in [Running hooks with run](#running-hooks-with-run). `option.queue.size` (default 2048) is the most runs that can wait; runs in progress do not count.
- A run that does not fit stops processing with `dispatch queue full (capacity <size>)` and runs global `onError` once, as described in [Event-loss notification](#event-loss-notification).
  - The watcher does not wait for room in the queue. This also holds for the runs made from events collected for [folder events](#folder-events), which can arrive many at once.

If `queue.size` is less than 1, the watcher runs no hook, prints an error such as the one below, and exits with code 1:

```text
Error: watchTargets[0].option.queue: size 0 is less than 1
```

`global.lifeCycle.onQueueWarning` is optional and uses the same command list and structured `dir`/`cmd` format as other hooks. It runs when the number of runs waiting in a target's queue moves between three states:

| Change | Runs when the waiting runs | `DW_QUEUE_LEVEL` | `DW_QUEUE_DIRECTION` |
|---|---|---|---|
| Normal to warning | reach 70% of `queue.size` | `70` | `up` |
| Warning to danger | reach 90% of `queue.size` | `90` | `up` |
| Danger to warning | fall below 80% of `queue.size` | `90` | `down` |
| Warning to normal | fall below 60% of `queue.size` | `70` | `down` |

- A state is left only 10 percentage points below the point where it was entered, so a count that moves up and down around 70% or 90% does not repeat the same warning.
- If one run passes two points, as with a very small `queue.size`, both warnings happen in order.
- The warning hook runs apart from event handling: events are still received and hooks still run while it runs.
- If warnings happen while the warning hook is still running for the same target, only the latest one is kept, and it runs after the current run ends. Each target keeps its own latest warning, so warning hooks of different targets can run at the same time.
- Each run of the warning hook has the same limits as `onError`: 5 seconds, followed by up to 20 seconds of cancellation cleanup.
- A failure or timeout is logged as `onQueueWarning notification failed: <cause>` and does not stop watching.
- A shutdown discards a kept warning and waits for a running warning hook to end.

The warning is passed as environment variables:

| Environment variable | Value |
|----------------------|-------|
| `DW_QUEUE_TARGET` | Watched target whose queue changed state |
| `DW_QUEUE_LEVEL` | `70` or `90` |
| `DW_QUEUE_DIRECTION` | `up` or `down` |
| `DW_QUEUE_LENGTH` | Number of runs waiting when the state changed |
| `DW_QUEUE_CAPACITY` | The target's `queue.size` |
| `DW_QUEUE_TOP_PATHS` | Up to 5 paths with the most waiting runs, most first, one per line in the form `<count> <path>`. Paths with the same count keep the order they were queued in. Each path has the same form as `{{.Path}}`. |

- `DW_QUEUE_TOP_PATHS` helps find a hook whose own output keeps running it again, as described in [Stopping a hook from rerunning itself](#stopping-a-hook-from-rerunning-itself).
- Read the variables with the shell's syntax, as for `onError`. File-event template variables are not available for this hook.
- A structured `dir` selects its working directory; without it, the command uses the program execution directory.

## Running hooks with run

- `option.run.concurrency` (default 1) is the most runs of a target that run at the same time. With the default, a target runs one run at a time.
- Runs start in the order they leave the queue, which is the order their events happened. Runs that run at the same time can end in any order.
- Runs of the same path, or of a folder and a path inside it, can also run at the same time. The `D` run of a file can therefore end before the earlier `C` run of the same file. Keep `concurrency: 1` if the hook depends on the order in which runs end.
- Each run runs global `onBeforeChange` and `onAfterChange` around its own `onChange`. If these hooks use something shared, such as one output file, runs at the same time can conflict.
- Whatever `concurrency` is, runs wait for target `onStartWatch` to end. Only with `interrupt: oldest` does the first run cancel a running target `onStartWatch`.

`option.run.interrupt` (default `none`) selects the running runs that a new run cancels:

| `interrupt` | What a new run cancels |
|---|---|
| `none` | Nothing. The new run waits for a free slot. |
| `samePath` | The run of the same path, even when a slot is free. If no run has the same path, the new run cancels nothing and waits for a free slot. |
| `oldest` | The run that started first, when every slot is taken. With `concurrency: 1`, this is the running run. |

- A new run starts only after the runs it canceled have ended.
- With `samePath`, hooks that do not end on their own, such as servers, can take every slot, and then runs of other paths wait until watching stops. Run such hooks with `concurrency: 1` and `interrupt: oldest`, so that every new run replaces the running one.

If `concurrency` is less than 1 or `interrupt` is not one of these values, the watcher runs no hook, prints an error such as the ones below, and exits with code 1:

```text
Error: watchTargets[0].option.run: concurrency 0 is less than 1
Error: watchTargets[0].option.run.interrupt: unknown value "true"; use none, samePath, or oldest
```

## Overload and shutdown

- Each target keeps at most `queue.size` pending runs, as described in [Queue size and warnings](#queue-size-and-warnings), and runs at most `run.concurrency` runs of its hooks at a time. `hookBatch.deliver: each` preserves received-event order; it does not become latest-only under load. Excluded paths neither make runs nor extend a wait. Creation-call order across files does not guarantee notification receipt order, so the last file created is not necessarily the path that `deliver: last` runs.
- A full dispatch queue or a watcher error stops processing. The default log includes the watched target and original cause; the CLI cleans up all targets and exits nonzero. Queue saturation is reported as `dispatch queue full`, distinct from an OS-reported overflow. No automatic restart or persistent queue is provided.
- A target that fails to start watching is also a fatal error: its `path` does not exist, its folder link cannot be resolved, or a folder cannot be added to the watch list. Because that target never ran `onStartWatch`, its `onFinishWatch` does not run either. The `onFinishWatch` of the other targets that started watching and global `onFinishWatch` still run, and the CLI exits with code 1.
- A shutdown signal cancels every running start and change run, discards pending changes, waits for event handling and every canceled run to stop, then runs finish hooks. Finish hooks keep their existing command behavior and have no additional timeout. Targets run independently.
- On Windows, a canceled hook gets CTRL+BREAK, and so do the programs it starts after the cancel, such as the next program of `a & b` in `cmd`. Processes of the hook that are still running 20 seconds after the first CTRL+BREAK are ended together, including processes that ignore the signal or use their own console or process group. Replacement and shutdown wait until every process of the hook has ended, so a hook that does not exit on CTRL+BREAK, including some `powershell` hooks, delays them by up to 20 seconds.
- On Windows, a hook canceled less than 100 ms after it starts gets CTRL+BREAK only when 100 ms have passed since its start, so replacement and shutdown right after a hook starts take up to 100 ms longer.
- On Windows, processes started by a hook that finishes without being canceled keep running.
- What the Windows E2E checks about stopping canceled hooks is listed in [Windows E2E](e2e/docs/windows.md#hook-cancellation-checks).
- Filesystem actions do not map one-to-one to notifications. Linux reports an OS event overflow, which stops processing like a full dispatch queue; macOS does not report one. Application bounds do not guarantee detection of every OS-level loss. See [E2E scenarios and limits](e2e/docs/reliability.md#race-and-overload-verification).

- The order of these steps is drawn in the [README diagrams](README.md#how-it-works).

## Default exclusion

- Files and folders whose names start with `.` or `_` are not watched. For example, creating, modifying, or deleting `.env`, or changing anything inside `.git` or `.idea`, does not run hooks. The folder set as `path` is watched regardless of its name.
- Links to folders inside the watched folder are not followed. Changes in a folder outside the watched folder do not run hooks, even when a link inside the watched folder points to it.

## Excluding paths with exclude

- `option.watch.exclude` lists path patterns.
- A file or folder that matches is not watched:
  - A matched folder and everything below it are not added to the watch list, whether the folder existed when watching started or was created later. Excluding a large folder such as `node_modules` adds no watches for anything in it.
  - Creating, modifying, or deleting a matched file does not run hooks, and creating or deleting a matched folder sends no [folder event](#folder-events).

```yaml
watchTargets:
  - path: site
    option:
      watch:
        exclude:
          - '**/node_modules/**'
          - '**/*.log'
          - '!important.log'
```

- Each pattern is matched against the path relative to the target's `path`, with `/` between folders on every OS. A pattern without `**` matches from the top of `path` only: `build.log` matches `site/build.log` but not `site/logs/build.log`.
- `*` matches any part of a name, `?` matches one character, and `**` matches any number of folders. `dist/**` matches the folder `dist` and everything below it, and `**/*.log` matches `.log` files at any depth. Character classes such as `[abc]` and alternatives such as `{js,css}` also work.
- Patterns are checked from top to bottom, and the last pattern that matches decides. A pattern that starts with `!` watches a path again that an earlier pattern excluded. In the example, `important.log` directly under `site` runs hooks, and other `.log` files do not.
- `!` cannot watch a path inside an excluded folder again, because that folder is not watched. To keep part of a folder, exclude the entries inside it instead of the folder: with `dist/*` and `!dist/keep.txt`, only `keep.txt` in `dist` runs hooks. With `dist/**` and `!dist/keep.txt`, `dist` is not watched, so `keep.txt` does not run hooks either.
- `dist/**` also matches the folder `dist` itself, so creating or deleting `dist` sends no folder event. `dist/*` matches only the entries in `dist`, so the hook still receives the folder events of `dist`.
- `!` also cannot watch a name that [default exclusion](#default-exclusion) excludes.
- Quote each pattern. Unquoted, YAML reads a value that starts with `*` or `!` as its own syntax.

- If a pattern is malformed, the watcher runs no hook, prints an error such as `Error: watchTargets[0].option.watch.exclude[1]: invalid pattern "[a"`, and exits with code 1.

### Stopping a hook from rerunning itself

- When a hook writes a file inside the target's `path`, that write is a change too, so the hook runs again, writes again, and keeps running.
- Add the path the hook writes to `watch.exclude`.

In this example, `onChange` writes `site/build.log`, and `exclude: ['build.log']` under `watch` stops that write from running the hook:

```yaml
watchTargets:
  - path: site
    lifeCycle:
      onChange:
        - dir: .
          cmd: make build > site/build.log
    option:
      watch:
        exclude:
          - 'build.log'
```

- With `run.interrupt: none`, the default, other files changed while the hook runs still run the hook after it finishes.

### Moving from excludeDir and excludeSuffix

- Version 3 removed `excludeDir` and `excludeSuffix`.

If a target's `option` still has either, the watcher runs no hook, prints the error below, and exits with code 1:

```text
Error: watchTargets[0].option.excludeDir is no longer supported; use option.watch.exclude (see USAGE "Moving from excludeDir and excludeSuffix")
```

Move each value to `watch.exclude`, keeping these differences in mind:

- An `excludeDir` value was a folder path relative to the directory the watcher runs in. An `exclude` pattern is relative to the target's `path`.
- An `excludeSuffix` value was compared with the end of the absolute path, so `out.txt` also excluded `layout.txt`. In `exclude`, write `**/` before the file name to match that name at any depth.
- A pattern moved from `excludeSuffix` that matches a folder name, such as `**/*~` for a folder `backup~`, now also excludes everything in that folder.

For a target with `path: 'site'`:

| Before | After |
|---|---|
| `excludeDir: ['site/dist']` | `watch: {exclude: ['dist/**']}` |
| `excludeSuffix: ['_test.go']` | `watch: {exclude: ['**/*_test.go']}` |
| `excludeSuffix: ['~']` | `watch: {exclude: ['**/*~']}` |
| `excludeSuffix: ['/out.txt']` | `watch: {exclude: ['**/out.txt']}` |

## Overlapping target paths

- When two watch targets have the same `path`, or one target's `path` is a parent folder of the other's, both targets watch the overlapping folders.
- The watcher runs such a config without an error or warning.

- One change in an overlapping folder runs the hooks of every target that watches that folder.
- For example, when you watch both `src` and `src/api`, creating `src/api/a.txt` runs the `onChange` of both targets.
- Global `onBeforeChange` and `onAfterChange` also run for each target, so the single change in that example runs each of them twice.
- The hooks of one target do not wait for the hooks of the other target to finish.
- Each target applies its own queue, `hookBatch.waitMillisecond` wait, and `run` concurrency and cancellation.
- Each target adds the overlapping folders to its own watch list, so they use OS watch resources, such as inotify watches on Linux or open files on macOS, once per target.
- When one target's hook writes a file inside another target's `path`, that write runs the other target's hooks.

- To keep a parent target from watching a child target's folder, add that folder to the parent target's `watch.exclude`.

With the config below, a change in `src/api` runs only the `src/api` target's hooks, and the `src` target does not add `src/api` to its watch list:

```yaml
watchTargets:
  - path: src
    option:
      watch:
        exclude:
          - 'api/**'
  - path: src/api
```

- The pattern is relative to the parent target's `path`, so write `api/**`, not `src/api/**`.

## Folder events

- The hook receives events of folders as well as files. `{{.IsDir}}` and `DW_IS_DIR` are `true` for a folder event and `false` for a file event.
- A folder event is `C` or `D`. A folder has no `U`: a change inside a folder reaches the hook as an event of the file inside it.
- `option.watch.fileEvent` chooses the file events that run the hook, and `option.watch.dirEvent` chooses the folder events. They default to `CUD` and `CD`, and `none` turns one of them off.

The hook receives these events for each action, written as event, `IsDir`, and path:

| Action | Events the hook receives |
|---|---|
| `mkdir src/pkg` | `C true src/pkg` |
| `mkdir -p src/a/b/c` | One `C true src/a` |
| Move a folder that holds files into the target as `src/x` | One `C true src/x` |
| `mkdir src/n && echo hi > src/n/a.txt` | One `C true src/n` |
| `rm -rf src/pkg` with 300 files inside | One `D true src/pkg` |
| Move `src/pkg` out of the target | One `D true src/pkg` |
| Rename a folder, or move it under another folder in the target | `D true` for the old path, then `C true` for the new path |
| Create, update, delete, or rename a file | The [file events](#renamed-and-moved-files), with `IsDir` `false` |

A hook can rely on the following:

- A folder `C` means that the folder appeared together with everything in it. The hook reads what is inside the folder itself: the watcher sends no file events for the files a folder brings with it, even when the folder is moved in.
- A folder `D` means that the folder and everything in it are gone.
- Events of the files and subfolders inside a folder that arrive while its folder event is being collected are absorbed into that folder event. They are absorbed in the same way with `watch.dirEvent: none`, so a change of a whole folder then runs no hook.
- Among the events collected in one wait, `hookBatch.deliver: lastPerPath` drops the `D` of each path under a folder whose final change is `D`. A folder deletion split across several collections of folder events therefore reaches the hook as one folder `D` when one wait collects it.
  - These `D`s are dropped in the same way with `watch.dirEvent: none`, so that folder deletion then runs no hook.
  - The `C` of a path under a folder `C` is not dropped, because the path may have appeared after the hook that received the folder `C` read the folder.
  - `last` passes only its last run, and `each` passes every split deletion as it is.
- With `hookBatch.deliver: each`, the hook receives the events of a target in the order they happened. While folder events are being collected, the file events of that target wait with them.
- The hook can receive the same change twice. For example, after the hook receives `C` for a folder and reads it, it can receive `C` for a file in that folder.

### Collecting folder events

- After a folder is created or deleted, the watcher collects the events of the target and sends them once no event has arrived for `dirBatch.waitMillisecond` (default 100).
- When events keep arriving, the watcher sends what it has collected `dirBatch.maxWaitMillisecond` (default 1000, or `dirBatch.waitMillisecond` if larger) after the first event. Events inside the folder after that reach the hook on their own and are not absorbed. `hookBatch.deliver: lastPerPath` still drops the `D`s under a folder `D` collected in the same wait.
- Deleting a file inside a subfolder of the target also starts collecting, because deleting a whole folder reports the files inside it before the folder. Even with `hookBatch.waitMillisecond: 0`, such a deletion and the events after it reach the hook up to `dirBatch.waitMillisecond` later.
- With `dirBatch.waitMillisecond: 0`, the watcher does not collect, so the hook also receives the events of the files and subfolders inside a folder that is created or deleted.
- If a written `dirBatch.maxWaitMillisecond` is less than `dirBatch.waitMillisecond`, the watcher runs no hook, prints an error such as `Error: watchTargets[0].option.dirBatch: maxWaitMillisecond 200 is less than waitMillisecond 500`, and exits with code 1.

If `watch.fileEvent` or `watch.dirEvent` is empty, has a letter it does not accept, or both are `none`, the watcher runs no hook, prints an error such as the one below, and exits with code 1:

```text
Error: watchTargets[0].option.watch.dirEvent: U is not a folder event; use C, D, or none
```

### Limitations on folder event detection

- If deleting a tree pauses for longer than `dirBatch.waitMillisecond`, for example on a slow disk or with a small `dirBatch.waitMillisecond`, a subfolder can reach the hook as its own `D` before the `D` of the folder above it. Raise `dirBatch.waitMillisecond` for such disks.
- A link to a folder is not followed, so creating or deleting the link is a file event with `IsDir` `false`.
- A folder that is deleted before the watcher handles its creation reaches the hook as a file `C` and `D`, because the watcher can no longer tell that it was a folder.
- After a folder is moved out of the target, changes in it no longer run hooks.
  - On Windows, the watcher re-creates its watches of the target about 100 ms after such a move, and changes made in the target at that moment can reach the hook twice.
- On Windows, renaming or moving a folder that holds a watched subfolder fails with `Access is denied` while the folder is watched.

### Moving from watchEvent

- Version 3 replaced `watchEvent` with `watch.fileEvent` and `watch.dirEvent`.

If a target's `option` still has `watchEvent`, the watcher runs no hook, prints the error below, and exits with code 1:

```text
Error: watchTargets[0].option.watchEvent is no longer supported; use option.watch.fileEvent and option.watch.dirEvent (see USAGE "Moving from watchEvent")
```

Move the value as follows:

| Before | After |
|---|---|
| `watchEvent: CU` | `watch: {fileEvent: CU}` |
| `watchEvent: ''` | `watch: {fileEvent: CUD}`, or no `fileEvent` |

- Folder events are on by default. A hook that expects the path of a file, such as `cat {{.Path}}`, has to check `{{.IsDir}}`, or the target has to set `watch.dirEvent: none`.
- Before version 3, the hook received these events for folders:
  - Creating a folder or moving one in ran no hook. A file written right after its folder was created sometimes ran the hook, and it is now absorbed into the folder `C`.
  - Deleting a folder ran the hook with `D` for the folder path, with `IsDir` unavailable, and on Linux and macOS also with `D` for each file inside. Now the hook receives one folder `D`.
  - These file events stay absorbed with `watch.dirEvent: none` as well.
