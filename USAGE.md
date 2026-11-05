# Directory Watcher usage guide

🇺🇸 English | 🇰🇷 [한국어](USAGE.ko.md)

- This guide assumes you have installed Directory Watcher as described in the [README](README.md#installation).
- To use a recipe, save its config as `config.yml` and run `directory-watcher` in the directory that holds it.

## Recipes

### Restart a dev server on every save

```yaml
watchTargets:
  - path: server
    lifeCycle:
      onStartWatch:
        - go run .
      onChange:
        - go run .
    option:
      run:
        interrupt: oldest
```

- The server starts when watching starts.
- With `interrupt: oldest`, each save stops the running server and the processes it started in the hook's process group (Unix) or Job Object (Windows), waits for them to stop, and then starts the server again. See [Running hooks with run](docs/guide/running.md#running-hooks-with-run).
  - Processes that ignore the stop signal are forcibly ended after about 20 seconds; processes that leave the group or job are outside this cleanup. See [Overload and shutdown](docs/guide/running.md#overload-and-shutdown).
- Hooks without `dir` run in the target's `path`, so `go run .` runs in `server`. See [Command execution location](docs/guide/configuration.md#command-execution-location).
- Ctrl+C stops the server and then the watcher.

### Run the tests of the package you saved

```yaml
watchTargets:
  - path: .
    lifeCycle:
      onChange:
        - if [ {{.ExtName}} = .go ]; then go test ./{{.ParentPath}}; fi
    option:
      watch:
        dirEvent: none
```

- `{{.ExtName}}` and `{{.ParentPath}}` are the extension and the folder of the changed file, so saving a `.go` file runs `go test` for the package that holds it, and saving other files runs nothing. See [Variables in commands](docs/guide/configuration.md#variables-in-commands).
- `dirEvent: none` keeps creating or deleting a folder from running the hook. See [Folder events](docs/guide/events.md#folder-events).
- When several files change at once, the hook runs once, for the file that changed last. To test every package that changed, set `hookBatch.deliver: lastPerPath`, which runs the hook once for each changed file. See [Waiting and merging with hookBatch](docs/guide/running.md#waiting-and-merging-with-hookbatch).
- A failing test is logged as an `[ERROR]` line.

### Copy changes to another folder

This recipe needs `rsync`:

```yaml
watchTargets:
  - path: src
    lifeCycle:
      onStartWatch:
        - dir: .
          cmd: rsync -a --delete src/ backup/
      onChange:
        - dir: .
          cmd: rsync -a --delete src/ backup/
```

- `onStartWatch` copies `src` to `backup` when watching starts, and `onChange` copies it again after each change, so `backup` keeps the same files as `src`.
- `--delete` removes files from `backup` that are no longer in `src`.
- `dir: .` runs `rsync` in the directory where you started the watcher. See [Command execution location](docs/guide/configuration.md#command-execution-location).
- Keep the destination outside `path`. A copy written inside `path` is a change too, so the hook would run again and again. See [Stopping a hook from rerunning itself](docs/guide/events.md#stopping-a-hook-from-rerunning-itself).
- Files whose names start with `.` or `_` are copied too, but changing only such files does not run the hook. See [Default exclusion](docs/guide/events.md#default-exclusion).

### Run the watcher in Docker

Mount the project directory at `/app` and pass the path of its `config.yml`:

```shell
docker run --rm \
  -v "$PWD:/app" \
  ghcr.io/seungyeop-lee/directory-watcher:latest -c /app/config.yml
```

In `config.yml`, write the paths under `app`:

```yaml
watchTargets:
  - path: app/src
    lifeCycle:
      onChange:
        - dir: app
          cmd: make build
```

- The container's working directory is `/`, so relative paths in `config.yml` resolve under `/`, and `app/src` is the `src` folder of the project.
- `make build` runs inside the container and can use only the tools in the image, such as `make`, `git`, and `curl`. To build with your own toolchain, add the watcher to an image that has it, as described in [Adding the watcher to your own image](docs/guide/installation.md#adding-the-watcher-to-your-own-image).
- [Running with Docker](docs/guide/installation.md#running-with-docker) lists the tools in the image and a known issue with deletions in bind-mounted directories.

## Guides

- [Installing and running](docs/guide/installation.md) covers installing with Homebrew, the command-line options, running with Docker, and running on Windows.
- [Configuration file](docs/guide/configuration.md) covers the full `config.yml` format, the shell that runs hook commands, where commands run, and the variables that commands can use.
- [Events and watched paths](docs/guide/events.md) covers the events of renamed and moved files and of folders, default exclusion, excluding paths with `exclude`, and overlapping target paths.
- [Running hooks](docs/guide/running.md) covers printing hook commands and dry runs, waiting and merging events with `hookBatch`, the queue size and its warnings, running hooks with `run`, event-loss notification, and overload and shutdown.
- [How it works](docs/guide/how-it-works.md) draws how the watcher handles changes and how it stops.
