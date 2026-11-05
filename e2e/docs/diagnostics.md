# E2E failure and timeout diagnostics and isolation

🇺🇸 English | 🇰🇷 [한국어](diagnostics.ko.md)

## Failure and timeout diagnostics

- When a local run fails, `config.yml`, `records.log`, `watcher.log`, `watcher.pid`, and `cleanup.log` are kept at the path after `diagnostics retained:`.
- To keep them on success as well, set `DW_E2E_KEEP=1`.
- Run folders are created under the OS temporary directory by default, and under `DW_E2E_DIAGNOSTICS` when it is set.
  - CI sets it to `$RUNNER_TEMP/dw-e2e` and uploads that directory as the `e2e-diagnostics-<OS>` artifact.
- In `cleanup.log`, check `alive=false` for the helper processes and check that the watcher PID has also exited.
- On Windows, `records.log.processes` from before cleanup is kept as well; see [Windows failure analysis](windows.md#failure-analysis) for how to analyze it.
- On Windows, `dll-init-failed.log` is kept as well when a process in that scenario exited with 0xc0000142. It holds the CTRL+BREAK sent in the test binary's console during the scenario and the related exits in time order, and the same report appears in the test output. How to read it is in [A hook exits with 0xc0000142](windows-utm.md#a-hook-exits-with-0xc0000142).

- A delete error for a run folder that is not kept fails that test, and a delete error for the test binary folder makes the final exit code 1.
- `TestCleanupReportsLockedResources` opens real delete-blocking handles on Windows and checks that neither error is hidden as success.
- When a run folder is kept because of a failure or `DW_E2E_KEEP=1`, deletion of that folder has not been verified.

- On Linux, `runner.log` and the diagnostics from the container's `/tmp` are copied to the host path after `Linux diagnostics:`.
  - The Linux runner passes `DW_E2E_KEEP=1` to the container, so the run folders of passed scenarios are copied as well.
- On a timeout, compare the `deadline` in the runner log with the last event and hook in the watcher log and the last `signal` and `complete` in the records.
- The container ID must no longer appear in `docker inspect <ID>`.

### Intentional failures and timeouts

- The commands below deliberately exercise the cleanup path, and **failure is the expected result**.
- Only [`TestProcessLifecycle`](scenarios/process-lifecycle.md) reads `DW_E2E_FAULT`.
- `failure` fails with `intentional failure after process tree readiness` right after the real process tree is ready, and `timeout` exceeds the time limit of a condition that is never met after readiness and fails with `timeout: intentional timeout`.
- In both cases, check the diagnostics path, PID exit, and removal of the Linux container.
- The Windows PowerShell commands are in [Windows failure analysis](windows.md#failure-analysis), and the UTM Windows VM command is in [Test selection, repetition, and time limits](windows-utm.md#test-selection-repetition-and-time-limits).

```sh
DW_E2E_FAULT=failure go test -count=1 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
DW_E2E_FAULT=timeout go test -count=1 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
DW_E2E_FAULT=failure go run ./e2e/linux -- -run '^TestProcessLifecycle$'
DW_E2E_FAULT=timeout go run ./e2e/linux -- -run '^TestProcessLifecycle$'
```

## Parallel runs across worktrees and isolation

- In separate terminals, move to each worktree root and run the commands in the [setup and run section of the E2E README](../README.md#setup-and-run).
- Local and Linux runs may run at the same time.

- Binaries, configuration, watch targets, and records are created in a temporary directory per run.
- The Linux runner copies the current files tracked by Git and new files that are not ignored. It reflects uncommitted modifications, additions, and deletions, and does not copy ignored files or the Git repository itself.
- Each container uses a new ID. There are no host bind mounts, shared volumes, or fixed container names, and building, file changes, and hook verification happen inside the container.
- After the container's test finishes or fails, or the runner's time limit is exceeded, the logs are copied and the container is removed. If you force-close the terminal during a run or kill the runner with SIGKILL, the cleanup code cannot run. In that case, run `docker rm --force <container ID>` with the container ID that was printed. Do not bulk-delete containers of other runs.
