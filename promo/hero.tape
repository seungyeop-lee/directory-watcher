# README의 hero GIF(hero.gif)를 녹화한다. promo/에서 make로 실행한다.
# 장면마다 scene.sh가 tmux를 띄우고, 왼쪽 아래 창에 입력한 명령에 오른쪽 창의 watcher가 반응한다.

Output hero.gif

Set Shell "bash"
Set FontSize 16
Set Width 1300
Set Height 540
Set Padding 16
Set Framerate 12
Set TypingSpeed 40ms
Set Theme "Catppuccin Mocha"

Hide
Type "./scene.sh start 1-restart '1/4  Restart your dev server on every save'" Enter
Wait+Screen@60s /listening/
Show
Sleep 1.5s
Type "vim server/main.go" Sleep 300ms Enter
Sleep 800ms
Type "/v1" Sleep 300ms Enter
Sleep 500ms
Type "cwv2" Sleep 300ms Escape
Sleep 500ms
Type ":w" Sleep 300ms Enter
Wait+Screen@30s /v2: listening/
Sleep 2.5s

Hide
Type ":q" Enter
Type '"$PROMO_SCENE" stop' Enter
Sleep 1s
Type "clear" Enter
Type "./scene.sh start 2-test '2/4  Run your tests on every save'" Enter
Wait+Screen@60s /watchTargets:/
Sleep 500ms
Show
Sleep 1s
Type "vim greet/greet.go" Sleep 300ms Enter
Sleep 800ms
Type "/Hello" Sleep 300ms Enter
Sleep 500ms
Type "cwHi" Sleep 300ms Escape
Sleep 500ms
Type ":w" Sleep 300ms Enter
Wait+Screen@30s /FAIL/
Sleep 2s
Type "u" Sleep 500ms
Type ":w" Sleep 300ms Enter
Wait+Screen@30s /ok +example/
Sleep 2.5s

Hide
Type ":q" Enter
Type '"$PROMO_SCENE" stop' Enter
Sleep 1s
Type "clear" Enter
Type "./scene.sh start 3-inbox '3/4  Process each file dropped into a folder'" Enter
Wait+Screen@60s /watchTargets:/
Sleep 500ms
Show
Sleep 1.5s
Type "cp orders/*.csv inbox/" Sleep 300ms Enter
Wait+Screen@30s /orders-wed.csv: 15 orders/
Sleep 3s

Hide
Type '"$PROMO_SCENE" stop' Enter
Sleep 1s
Type "clear" Enter
Type "./scene.sh start 4-forward '4/4  Forward file events to another service'" Enter
Wait+Screen@60s /listening/
Sleep 500ms
Show
Sleep 1.5s
Type 'echo "# Setup" > docs/setup.md' Sleep 300ms Enter
Wait+Screen@30s /received C docs.setup.md/
Sleep 1.5s
Type "rm docs/draft.md" Sleep 300ms Enter
Wait+Screen@30s /received D docs.draft.md/
Sleep 3s

Hide
Type '"$PROMO_SCENE" stop' Enter
Sleep 1s
