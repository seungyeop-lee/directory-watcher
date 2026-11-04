# Installing and running

🇺🇸 English | 🇰🇷 [한국어](installation.ko.md)

## Installing with Homebrew

- The cask works on macOS and Linux.
- It needs Homebrew 5.1.14 or later.
- With an older Homebrew, installing the cask fails with `Unexpected method 'postflight_steps'`.

Check the version with `brew --version`, and if it is older, update Homebrew first:

```shell
brew update
```

Then install the cask:

```shell
brew install --cask seungyeop-lee/tap/directory-watcher
```

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

### Adding the watcher to your own image

Copy the executable from the published image in your Dockerfile:

```dockerfile
COPY --from=ghcr.io/seungyeop-lee/directory-watcher:latest /directory-watcher /usr/local/bin/
```

## Running on Windows

- Output may not display correctly in PowerShell or Command Prompt. Run the watcher in Git Bash instead.
- When the watcher stops a hook, it ends the hook's processes that do not exit on CTRL+BREAK only 20 seconds later, so replacing or stopping such a hook, including some `powershell` hooks, can take up to 20 seconds. See [Overload and shutdown](running.md#overload-and-shutdown).
