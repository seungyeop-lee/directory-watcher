# Running hooks

🇺🇸 English | 🇰🇷 [한국어](running.ko.md)

## Printing hook commands

- Before it runs each hook command, the watcher prints a line with the hook name and the command, with the same prefix as the hook's output.
  - In hooks that run on a change, the line shows the command with its template variables filled in.
  - For a command with a `dir`, the line ends with `(dir: <dir>)`.
  - The line has no log prefix such as `[INFO]`, and `--log-level` does not change it.

With the configuration below, saving `server/main.go` prints the following lines:

```yaml
watchTargets:
  - path: server
    lifeCycle:
      onChange:
        - echo changed {{.Name}}
        - dir: web
          cmd: echo building
```

```text
server      | onChange: echo changed main.go
server      | changed main.go
server      | onChange: echo building (dir: web)
server      | building
```

- `-s`, `--silent` turns off the command lines and the `config: <path>` line that names the [config file](configuration.md#configyml) read at startup. The output of the hook commands still appears.
- `-n`, `--dry-run` prints the command lines without running the commands.
  - The watcher still watches the targets and runs every hook, including `onError` and `onQueueWarning`, but skips the commands themselves.
  - Before it skips a command, the watcher builds it and checks its working directory, so a missing working directory is reported as in a real run.
- Because a skipped command ends at once, a dry run does not show how commands behave over time:
  - Runs rarely overlap, so `run.concurrency` and `run.interrupt` have almost no visible effect, and the queue rarely fills, so queue warnings and the full-queue error rarely occur.
  - A skipped command never fails, so the commands after one that would fail are printed instead of being skipped.

`-s` and `-n` cannot be used together. The watcher then runs no hook, prints the error below with the usage, and exits with code 1:

```text
Error: if any flags in the group [silent dry-run] are set none of the others can be; [dry-run silent] were all set
```

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

## Queue size and warnings

- The runs of a target wait in the target's queue until they start, as described in [Running hooks with run](#running-hooks-with-run). `option.queue.size` (default 2048) is the most runs that can wait; runs in progress do not count.
- A run that does not fit stops processing with `dispatch queue full (capacity <size>)` and runs global `onError` once, as described in [Event-loss notification](#event-loss-notification).
  - The watcher does not wait for room in the queue. This also holds for the runs made from events collected for [folder events](events.md#folder-events), which can arrive many at once.

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

- `DW_QUEUE_TOP_PATHS` helps find a hook whose own output keeps running it again, as described in [Stopping a hook from rerunning itself](events.md#stopping-a-hook-from-rerunning-itself).
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

## Overload and shutdown

- Each target keeps at most `queue.size` pending runs, as described in [Queue size and warnings](#queue-size-and-warnings), and runs at most `run.concurrency` runs of its hooks at a time. `hookBatch.deliver: each` preserves received-event order; it does not become latest-only under load. Excluded paths neither make runs nor extend a wait. Creation-call order across files does not guarantee notification receipt order, so the last file created is not necessarily the path that `deliver: last` runs.
- A full dispatch queue or a watcher error stops processing. The default log includes the watched target and original cause; the CLI cleans up all targets and exits nonzero. Queue saturation is reported as `dispatch queue full`, distinct from an OS-reported overflow. No automatic restart or persistent queue is provided.
- A target that fails to start watching is also a fatal error: its `path` does not exist, its folder link cannot be resolved, or a folder cannot be added to the watch list. Because that target never ran `onStartWatch`, its `onFinishWatch` does not run either. The `onFinishWatch` of the other targets that started watching and global `onFinishWatch` still run, and the CLI exits with code 1.
- A shutdown signal cancels every running start and change run, discards pending changes, waits for event handling and every canceled run to stop, then runs finish hooks. Finish hooks keep their existing command behavior and have no additional timeout. Targets run independently.
- On Windows, a canceled hook gets CTRL+BREAK, and so do the programs it starts after the cancel, such as the next program of `a & b` in `cmd`. Processes of the hook that are still running 20 seconds after the first CTRL+BREAK are ended together, including processes that ignore the signal or use their own console or process group. Replacement and shutdown wait until every process of the hook has ended, so a hook that does not exit on CTRL+BREAK, including some `powershell` hooks, delays them by up to 20 seconds.
- On Windows, a hook canceled less than 500 ms after it starts gets CTRL+BREAK only when 500 ms have passed since its start, so replacement and shutdown right after a hook starts take up to 500 ms longer.
  - If the hook is canceled less than 500 ms after it starts a program, the hook and that program also get CTRL+BREAK only when 500 ms have passed since the program started.
- On Windows, processes started by a hook that finishes without being canceled keep running.
- What the Windows E2E checks about stopping canceled hooks is listed in [Windows E2E](../../e2e/docs/windows.md#hook-cancellation-checks).
- Filesystem actions do not map one-to-one to notifications. Linux and Windows report an OS event overflow, which stops processing like a full dispatch queue; macOS does not report one. Application bounds do not guarantee detection of every OS-level loss. See [E2E scenarios and limits](../../e2e/docs/reliability.md#race-and-overload-verification), which distinguishes the observed Linux overflow from the Windows backend contract.

- The order of these steps is drawn in [How it works](how-it-works.md).
