# inbox/에 들어온 CSV 파일 하나의 주문 수를 센다. 첫 줄은 머리글이라 빼고 센다.
sleep 0.3
echo "imported $1: $(( $(wc -l < "$1") - 1 )) orders"
