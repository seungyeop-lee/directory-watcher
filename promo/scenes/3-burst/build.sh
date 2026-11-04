# src/의 파일을 dist/app.js 하나로 합치고, 몇 번째 빌드인지 함께 출력한다.
mkdir -p dist
count=$(( $(cat dist/count 2>/dev/null || echo 0) + 1 ))
echo "$count" > dist/count
cat src/*.js > dist/app.js
echo "build #$count: bundled $(ls src | wc -l | tr -d ' ') files into dist/app.js"
