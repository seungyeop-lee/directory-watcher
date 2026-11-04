// 변경 이벤트를 받는 다른 서비스 역할을 한다. 요청 본문 "<이벤트>:<경로>"를 받을 때마다 한 줄씩 출력한다.
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		event, path, _ := strings.Cut(string(body), ":")
		fmt.Printf("received %s %s\n", event, path)
	})
	fmt.Println("receiver: listening on :8091")
	log.Fatal(http.ListenAndServe(":8091", nil))
}
