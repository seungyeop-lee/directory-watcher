# main 브랜치에 src/ 파일 50개를 커밋하고, 50개를 모두 고친 feature 브랜치를 만든다.
# 장면에서 git switch feature를 하면 파일 50개가 한꺼번에 바뀐다.
set -e
write_modules() {
	mkdir -p src
	for i in $(seq -w 1 50); do
		echo "export const m$i = $1;" > "src/m$i.js"
	done
}
commit() {
	git add src
	git -c user.name=demo -c user.email=demo@example.com commit -q -m "$1"
}

git init -q -b main
write_modules 1
commit "Add modules"
git switch -q -c feature
write_modules 2
commit "Update all modules"
git switch -q main
