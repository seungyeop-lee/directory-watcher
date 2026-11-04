#!/bin/sh
# hero.tape이 장면마다 부르는 스크립트다.
#   scene.sh start <장면 폴더> <제목>: 장면을 .work/에 복사하고, tmux 왼쪽 위 창에 config.yml을,
#     오른쪽 창에 directory-watcher를 띄운 뒤 그 tmux에 붙는다. 녹화하는 명령은 왼쪽 아래 창에 입력된다.
#   scene.sh stop: 오른쪽 창의 watcher를 Ctrl+C로 끝내고 tmux를 닫는다. 장면의 tmux 안에서 부른다.
set -eu

promo=$(cd "$(dirname "$0")" && pwd)
tmux_cmd="tmux -L dwpromo -f $promo/tmux.conf"
config=dw:0.0
shell=dw:0.1
right=dw:0.2

# 창의 셸이 프롬프트를 띄울 때까지 기다린다. 그 전에 보낸 키는 프롬프트 앞에 한 번 더 찍힌다.
wait_prompt() {
	for _ in $(seq 50); do
		$tmux_cmd capture-pane -p -t "$1" | grep -q '^\$ *$' && return
		sleep 0.1
	done
}

start() {
	scene=$1
	title=$2
	work=$promo/.work/$scene

	rm -rf "$work"
	mkdir -p "$promo/.work"
	cp -R "$promo/scenes/$scene" "$work"
	if [ -f "$work/setup.sh" ]; then
		(cd "$work" && sh setup.sh && rm setup.sh)
	fi

	# 장면 속 셸이 사용자 설정 대신 이 값을 쓰도록 tmux 서버를 띄우기 전에 정한다.
	export PATH="$promo/.work/bin:$PATH"
	export PS1='$ '
	export GIT_CONFIG_GLOBAL=/dev/null
	export GIT_PAGER=cat
	export BASH_SILENCE_DEPRECATION_WARNING=1
	export PROMO_SCENE="$promo/scene.sh"
	export VIMINIT="source $promo/vimrc"

	cols=$(tput cols)
	rows=$(tput lines)
	$tmux_cmd new-session -d -s dw -c "$work" -x "$cols" -y "$rows"
	$tmux_cmd set-option -g status-left " $title "
	# 왼쪽 창은 화면 너비의 45%로 두되, config.yml의 가장 긴 줄이 줄바꿈되지 않도록 필요하면 넓힌다.
	left_cols=$(awk -v w="$((cols * 45 / 100))" '{ if (length + 1 > w) w = length + 1 } END { print w }' "$work/config.yml")
	$tmux_cmd split-window -h -l "$((cols - left_cols - 1))" -t "$config" -c "$work"
	# 왼쪽 위 창에는 "$ cat config.yml" 줄, config.yml, 프롬프트 줄이 들어갈 높이만 남기고 나머지를 아래 창에 준다.
	# 상태 줄과 창 경계도 한 줄씩 차지하므로 config.yml 줄 수에 4를 더해 뺀다.
	$tmux_cmd split-window -v -l "$((rows - $(wc -l < "$work/config.yml") - 4))" -t "$config" -c "$work"
	wait_prompt "$right"
	$tmux_cmd send-keys -t "$right" 'directory-watcher' Enter
	wait_prompt "$config"
	$tmux_cmd send-keys -t "$config" 'cat config.yml' Enter
	wait_prompt "$shell"
	$tmux_cmd select-pane -t "$shell"
	$tmux_cmd attach-session -t dw
}

stop() {
	$tmux_cmd send-keys -t "$right" C-c
	# watcher가 끝나 오른쪽 창에 셸만 남을 때까지 최대 10초 기다린다.
	for _ in $(seq 50); do
		[ "$($tmux_cmd display-message -p -t "$right" '#{pane_current_command}')" = bash ] && break
		sleep 0.2
	done
	$tmux_cmd kill-server
}

case "${1:-}" in
start) start "$2" "$3" ;;
stop) stop ;;
*)
	echo "usage: $0 start <scene> <title> | stop" >&2
	exit 2
	;;
esac
