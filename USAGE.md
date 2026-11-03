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

```shell
docker run --rm ghcr.io/seungyeop-lee/directory-watcher:latest
```

**Known issue: delete events with bind mounts.** When you delete files on the host in a directory that is bind-mounted into a container, the delete event may not always be detected. This is a known issue with the underlying file system notification library (fsnotify). For details, see [fsnotify/fsnotify#592](https://github.com/fsnotify/fsnotify/issues/592).

## config.yml

The `config.yml` file defines the directories to watch and the commands to run for changes in each directory. You can also define commands to run when watching starts or stops. For a practical example, see [`config.example.yml`](config.example.yml). [`directory-watcher.schema.json`](directory-watcher.schema.json) describes the same format as a JSON schema. Running `make run` from the repository root first creates the folders this example watches and then starts the watcher with the example.

```yaml
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
      excludeDir:
        - [ path of the directory to exclude from monitoring ]
      excludeSuffix:
        - [ file suffix to exclude from monitoring ]
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

`<target>` is the last element of the target's `path`. A command stopped because a new change cancelled its run (`interruptible: true`) or because watching stopped is not a failure and is not logged.

### Command execution location

| dir | global onStartWatch, global onFinishWatch, global onError | Other hooks |
|-----|-------------------------------------------|-------------------------|
| O   | Specified dir location                    | Specified dir location  |
| X   | Program execution location                | Specified path location |

### Variables in commands

| Variable        | Description                                             | Example (when `test/dir2` is set as the monitored directory) |
|-----------------|---------------------------------------------------------|--------------------------------------------------------------|
| {{.Path}}       | Relative path of the file where the event occurred      | test/dir2/b.txt                                              |
| {{.AbsPath}}    | Absolute path of the file where the event occurred      | /Users/example/directory-watcher/test/dir2/b.txt             |
| {{.FileName}}   | File name where the event occurred                      | b.txt                                                        |
| {{.ExtName}}    | Extension of the file where the event occurred          | .txt                                                         |
| {{.DirPath}}    | Relative path of the directory where the event occurred | test/dir2                                                    |
| {{.DirAbsPath}} | Absolute path of the directory where the event occurred | /Users/example/directory-watcher/test/dir2                   |
| {{.Event}}      | Event type (C: create, U: update, D: delete)            | C                                                            |

If `path` is a link to a folder, the folder it points to is watched. The variables above then hold paths based on the fully resolved path instead of `path`. An `excludeDir` written under the link path excludes the same location in the linked folder.

## waitMillisecond and noWait

- When an event occurs within `watchTargets.path`, the hook is executed after waiting for waitMillisecond.
- If another event occurs within `watchTargets.path` during the waiting period, the previous event is ignored. That is, the waitMillisecond is reset based on the new event, and the hook is executed after the new waiting period.
- If you want the hook to be executed immediately for every event within `watchTargets.path` without ignoring any events, set noWait to true. (Hooks for events are executed sequentially.)

## Event-loss notification

`global.lifeCycle.onError` is optional and uses the same command list and structured `dir`/`cmd` format as other hooks. It runs at most once per runner, only when the first fatal error satisfies `errors.Is(err, fsnotify.ErrEventOverflow)`. This includes dispatch queue saturation and OS-reported overflow. Other fatal errors, normal termination, and debounce replacement do not notify.

Error context is passed as environment variables, never interpolated into command templates:

| Environment variable | Value |
|----------------------|-------|
| `DW_ERROR_TARGET` | Watched target associated with the fatal error |
| `DW_ERROR_KIND` | `event_loss` |
| `DW_ERROR_CAUSE` | Original cause string |
| `DW_ERROR_MESSAGE` | Complete `FatalError.Error()` message |
| `DW_ERROR_POSSIBLE_LOSS` | `true` (possible loss, not a lost-event count) |

Have the notification executable read these environment variables directly; command strings do not perform shell expansion or redirection. If you use an executable shell script, quote expansions inside it, for example `printf '%s\n' "$DW_ERROR_MESSAGE"`. File-event template variables are not available for this hook. A structured `dir` selects its working directory; without it, the command uses the program execution directory.

The first fatal error immediately prevents new normal-hook approvals across all targets. The notification has an independent, fixed 5-second execution timeout, followed by up to 20 seconds of cancellation cleanup (at most 25 seconds total), regardless of normal-hook cancellation. A notification failure or timeout is logged and never replaces the original fatal error; the CLI still exits nonzero for that original error. Normal finish hooks still run after cancellation, with their existing behavior.

After each error-notification command completes, fails, or times out, its remaining descendants are terminated within the same cleanup budget. On Windows, each notification command is assigned to a private Job Object before its suspended parent is allowed to run; cleanup does not depend on that parent's PID still being alive. Failure to establish that job prevents the notification command from running. Normal hooks and other executions are not assigned to this notification job.

## Overload and shutdown

- Each target admits at most 256 pending events and one active hook generation. `noWait: true` preserves received-event order; it does not become latest-only under load. Debounce retains the last received valid event, and excluded suffixes neither execute nor reset its timer. Creation-call order across files does not guarantee notification receipt order, so the last file created is not necessarily the debounce result.
- A full dispatch queue or a watcher error stops processing. The default log includes the watched target and original cause; the CLI cleans up all targets and exits nonzero. Queue saturation is reported as `dispatch queue full`, distinct from an OS-reported overflow. No automatic restart or persistent queue is provided.
- Interruptible replacement cancels and waits for the previous generation before starting another. A shutdown signal cancels active startup/change commands, discards pending changes, joins event handlers, then runs finish hooks. Finish hooks keep their existing command behavior and have no additional timeout. Targets run independently.
- Path checks close any files they open before returning, including when directory metadata lookup fails. Releasing these handles does not depend on garbage collection.
- Filesystem actions do not map one-to-one to notifications. In fsnotify v1.7.0, inotify reports `ErrEventOverflow`; kqueue on macOS does not report that sentinel. Application bounds do not guarantee detection of every OS-level loss. See [E2E scenarios and limits](e2e/docs/reliability.md#race-and-overload-verification).

The order of these steps is drawn in the [README diagrams](README.md#how-it-works).

## Default exclusion

- Files and folders whose names start with `.` or `_` are not watched. For example, creating `.env` or changing anything inside `.git` or `.idea` does not run hooks. The folder set as `path` is watched regardless of its name.
- Files are excluded only when they are created. Modifying or deleting an existing `.env` runs hooks.
- Links to folders inside the watched folder are not followed. Changes in a folder outside the watched folder do not run hooks, even when a link inside the watched folder points to it.

## Limitations on folder event detection

- By default, hooks are executed based on **file** events within the monitored folder.
- Therefore, hooks are not executed for folder creation or renaming.
- However, hooks are executed for folder deletion (including empty folders) as it cannot be recognized that the target is a folder.
