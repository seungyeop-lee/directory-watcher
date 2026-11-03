#!/bin/sh
# config.example.yml로 watcher를 실행해, 모든 대상의 감시가 시작되고 파일을 만들면 onChange 훅이,
# SIGTERM을 보내면 onFinishWatch 훅이 실행된 뒤 종료 코드 0으로 끝나는지 확인한다.
# 저장소 루트에서 `make smoke-example`로 실행한다. 인자는 감시 폴더 목록이며, Makefile이
# example-watch-dirs로 만든 EXAMPLE_WATCH_DIRS를 넘긴다.
# 확인하는 훅 출력은 config.example.yml의 훅 명령과 맞춰 둔 것이라, 예시의 훅을 바꾸면 여기도 고친다.
set -eu

root=$(pwd)
work=$(mktemp -d)
log=$work/watcher.log
file=smoke-example.txt
pid=

cleanup() {
	if [ -n "$pid" ]; then kill -KILL "$pid" 2>/dev/null || true; fi
	for dir in "$@"; do rm -f "$dir/$file"; done
	rm -rf "$work"
}
# shellcheck disable=SC2064
trap "cleanup $*" EXIT

fail() {
	echo "smoke-example: FAIL: $1" >&2
	echo "--- watcher output ---" >&2
	cat "$log" >&2
	exit 1
}

# log_has는 watcher 출력에 $1로 끝나는 줄이 있는지 본다. 줄 앞에는 색상 코드와 출력 주체가 붙으므로
# 줄 끝으로 비교한다.
log_has() {
	awk -v s="$1" 'length($0) >= length(s) && substr($0, length($0) - length(s) + 1) == s { found = 1 } END { exit !found }' "$log"
}

# wait_for는 $1로 끝나는 줄이 나올 때까지 10초 기다린다. 그 전에 watcher가 끝나면 바로 실패한다.
wait_for() {
	tries=0
	until log_has "$1"; do
		kill -0 "$pid" 2>/dev/null || fail "watcher exited before printing: $1"
		tries=$((tries + 1))
		[ "$tries" -le 100 ] || fail "timed out waiting for: $1"
		sleep 0.1
	done
	echo "smoke-example: ok: $1"
}

go build -o "$work/directory-watcher" .
"$work/directory-watcher" -c config.example.yml -l INFO >"$log" 2>&1 &
pid=$!

for dir in "$@"; do
	wait_for "watch start: $dir"
done

for dir in "$@"; do
	: >"$dir/$file"
done
wait_for "'dir1 onChange'"
wait_for "'dir2 onChange test/dir2/$file, $root/test/dir2/$file, $file, .txt, test/dir2, $root/test/dir2, C'"
wait_for "$root/cmd"

kill -TERM "$pid"
tries=0
while kill -0 "$pid" 2>/dev/null; do
	tries=$((tries + 1))
	[ "$tries" -le 100 ] || fail "watcher did not exit within 10s after SIGTERM"
	sleep 0.1
done
status=0
wait "$pid" || status=$?
pid=
[ "$status" -eq 0 ] || fail "watcher exited with status $status after SIGTERM"
echo "smoke-example: ok: exit status 0 after SIGTERM"
for line in "'dir2 onFinishWatch'" "$root/internal" "'global onFinishWatch'"; do
	log_has "$line" || fail "missing onFinishWatch output: $line"
	echo "smoke-example: ok: $line"
done

echo "smoke-example: PASS"
