# Configuration file

🇺🇸 English | 🇰🇷 [한국어](configuration.ko.md)

## config.yml

- The `config.yml` file defines the directories to watch and the commands to run for changes in each directory.
- You can also define commands to run when watching starts or stops.
- Without `-c`, `--config-path`, the watcher reads `config.directory-watcher.yml` in the working directory, or `config.yml` if that file does not exist.
  - If neither file exists, the watcher prints `Error: no config file found: tried config.directory-watcher.yml, config.yml (use -c to set a path)` and exits with code 1.
  - With `-c`, the watcher reads only the given path, so it exits with an error if that file does not exist, even when one of the two files does.
- When the watcher reads a config file, it prints a `config: <path>` line on standard output before it checks the settings.
  - Errors in the settings do not name the file, so this line tells you which file an error comes from.
  - `<path>` is the file name the watcher found, or the value given to `-c` as it is.
  - The line has no log prefix such as `[INFO]`, and `--log-level` does not change it.
  - `-s`, `--silent` turns the line off, and `-n`, `--dry-run` still prints it.
- For a practical example, see [`config.example.yml`](../../config.example.yml).
- [`directory-watcher.schema.json`](../../directory-watcher.schema.json) describes the same format as a JSON schema.
- Running `make run` from the repository root first creates the folders this example watches and then starts the watcher with the example.
- The first line of the block below, `# yaml-language-server: $schema=...`, links the file to the JSON schema above. Put it at the top of your `config.yml` too.
  - Editors that use yaml-language-server, such as VS Code with the Red Hat YAML extension, then complete keys and mark keys and values the schema does not allow.
  - The URL points to the branch named after the major version, such as `v3`, which holds the schema of that major's releases.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/seungyeop-lee/directory-watcher/v3/directory-watcher.schema.json
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

- Relative paths in `path` and `dir` are both resolved against the directory where you started the watcher (the launch directory).
  - `dir` is resolved against the launch directory too, not against `path`.
- A command with `dir` runs in that `dir`, whatever the hook.
- A command without `dir` runs in the directory below, depending on the hook.

| Hook | Directory when `dir` is omitted |
|------|---------------------------------|
| Target `onStartWatch`, `onChange`, `onFinishWatch` | That target's `path` |
| Global `onBeforeChange`, `onAfterChange` | The `path` of the target where the change happened |
| Global `onStartWatch`, `onFinishWatch`, `onError`, `onQueueWarning` | The launch directory |

Each command in the config below runs in the directory given in its comment, where `<launch dir>` is the directory where you started the watcher.

```yaml
watchTargets:
  - path: server
    lifeCycle:
      onChange:
        - go run .          # <launch dir>/server
  - path: web/src
    lifeCycle:
      onChange:
        - dir: web          # <launch dir>/web (not <launch dir>/web/src/web)
          cmd: npm run build
```

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
| {{.IsDir}}         | `DW_IS_DIR`          | `true` for a [folder event](events.md#folder-events), `false` for a file event               | false                                                        |

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

- A command can only insert a variable of the table as it is, as in `{{.Path}}` or `{{$.Path}}`.
- The watcher passes the shell a command in which each variable is replaced with a shell reference, so a condition or a function in the template would see the reference text, not the value.

If a command uses a condition such as `{{if eq .Event "C"}}` (`if`, `with`, `range`), a function such as `{{len .Path}}`, or a pipeline such as `{{.Path | printf "%q"}}`, the watcher runs no hook, prints an error like the one below, and exits with code 1:

```text
Error: watchTargets[0].lifeCycle.onChange[0]: unsupported template action {{len .Path}}; only template variables such as {{.Path}} are supported
```

- To run a command differently by value, write the condition in the shell command. In `sh`, write `if [ {{.Event}} = C ]; then echo created; fi`.

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
