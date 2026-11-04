# README의 hero GIF(hero.gif)를 녹화한다. promo/에서 make로 실행한다.
# 장면마다 scene.sh가 tmux를 띄우고, 왼쪽 창에 입력한 명령에 오른쪽 창의 watcher가 반응한다.

Output hero.gif

Set Shell "bash"
Set FontSize 16
Set Width 1280
Set Height 400
Set Padding 16
Set Framerate 12
Set TypingSpeed 40ms
Set Theme "Catppuccin Mocha"

Hide
Type "./scene.sh start 1-restart '1/3  Restart your dev server on every save'" Enter
Wait+Screen@60s /listening/
Show
Sleep 1.5s
Type "perl -pi -e 's/v1/v2/' server/main.go" Sleep 300ms Enter
Wait+Screen@30s /v2: listening/
Sleep 2.5s

Hide
Type '"$PROMO_SCENE" stop' Enter
Sleep 1s
Type "clear" Enter
Type "./scene.sh start 2-test '2/3  Run your tests on every save'" Enter
Wait+Screen@60s /watchTargets:/
Sleep 500ms
Show
Sleep 1s
Type "perl -pi -e 's/Hello/Hi/' greet/greet.go" Sleep 300ms Enter
Wait+Screen@30s /FAIL/
Sleep 2s
Type "perl -pi -e 's/Hi/Hello/' greet/greet.go" Sleep 300ms Enter
Wait+Screen@30s /ok +example/
Sleep 2.5s

Hide
Type '"$PROMO_SCENE" stop' Enter
Sleep 1s
Type "clear" Enter
Type "./scene.sh start 3-burst '3/3  Run once when 50 files change at once'" Enter
Wait+Screen@60s /watchTargets:/
Sleep 500ms
Show
Sleep 1s
Type "git diff --shortstat feature" Sleep 300ms Enter
Sleep 1.5s
Type "git switch feature" Sleep 300ms Enter
Wait+Screen@30s /build #1/
Sleep 3s

Hide
Type '"$PROMO_SCENE" stop' Enter
Sleep 1s
