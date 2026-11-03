# E2E failure and timeout diagnostics and isolation

🇺🇸 English | 🇰🇷 [한국어](diagnostics.ko.md)

## Failure and timeout diagnostics

When a local run fails, `config.yml`, `records.log`, `watcher.log`, `watcher.pid`, and `cleanup.log` are kept at the path after `diagnostics retained:`. To keep them on success as well, set `DW_E2E_KEEP=1`. In `cleanup.log`, check `alive=false` for the helper processes and check that the watcher PID has also exited. On Windows, `records.log.processes` from before cleanup is kept as well; see [Windows failure analysis](windows.md#failure-analysis) for how to analyze it.

A delete error for a run folder that is not kept fails that test, and a delete error for the test binary folder makes the final exit code 1. `TestCleanupReportsLockedResources` opens real delete-blocking handles on Windows and checks that neither error is hidden as success. When a run folder is kept because of a failure or `DW_E2E_KEEP=1`, deletion of that folder has not been verified.

On Linux, `runner.log` and the diagnostics from the container's `/tmp` are copied to the host path after `Linux diagnostics:`. On a timeout, compare the `deadline` in the runner log with the last event and hook in the watcher log and the last `signal` and `complete` in the records. The container ID must no longer appear in `docker inspect <ID>`.

### Intentional failures and timeouts

The commands below deliberately exercise the cleanup path, and **failure is the expected result**. `failure` fails right after the real process tree is ready, and `timeout` exceeds the time limit of a condition that is never met after readiness. In both cases, check the diagnostics path, PID exit, and removal of the Linux container. The Windows PowerShell commands are in [Windows failure analysis](windows.md#failure-analysis).

```sh
DW_E2E_FAULT=failure go test -count=1 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
DW_E2E_FAULT=timeout go test -count=1 -v -run '^TestProcessLifecycle$' ./e2e/scenarios
DW_E2E_FAULT=failure go run ./e2e/linux -- -run '^TestProcessLifecycle$'
DW_E2E_FAULT=timeout go run ./e2e/linux -- -run '^TestProcessLifecycle$'
```

## Parallel runs across worktrees and isolation

In separate terminals, move to each worktree root and run the commands in the [setup and run section of the E2E README](../README.md#setup-and-run). Local and Linux runs may run at the same time.

- Binaries, configuration, watch targets, and records are created in a temporary directory per run.
- The Linux runner copies the current files tracked by Git and new files that are not ignored. It reflects uncommitted modifications, additions, and deletions, and does not copy ignored files or the Git repository itself.
- Each container uses a new ID. There are no host bind mounts, shared volumes, or fixed container names, and building, file changes, and hook verification happen inside the container.
- After the container's test finishes or fails, or the runner's time limit is exceeded, the logs are copied and the container is removed. If you force-close the terminal during a run or kill the runner with SIGKILL, the cleanup code cannot run. In that case, run `docker rm --force <container ID>` with the container ID that was printed. Do not bulk-delete containers of other runs.
