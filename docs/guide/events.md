# Events and watched paths

🇺🇸 English | 🇰🇷 [한국어](events.ko.md)

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

## Folder events

- The hook receives events of folders as well as files. `{{.IsDir}}` and `DW_IS_DIR` are `true` for a folder event and `false` for a file event.
- A folder event is `C` or `D`. A folder has no `U`: a change inside a folder reaches the hook as an event of the file inside it.
- `option.watch.fileEvent` chooses the file events that run the hook, and `option.watch.dirEvent` chooses the folder events. They default to `CUD` and `CD`, and `none` turns one of them off.
- A hook that expects the path of a file, such as `cat {{.Path}}`, has to check `{{.IsDir}}` in the shell, as in `if [ {{.IsDir}} = false ]; then cat {{.Path}}; fi`, or the target has to set `watch.dirEvent: none`.

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
