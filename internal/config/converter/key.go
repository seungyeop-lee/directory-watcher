package converter

import "fmt"

// WithKey는 설정 키 key 아래에서 난 오류 err 앞에 key와 점을 붙인다. converter와 yaml의 빌드 함수는 모두 자기가 맡은 키로
// 시작하는 오류를 돌려주고, 아래 키에서 난 오류는 이 함수로 감싸 돌려준다. 그래서 오류는 설정 파일의 맨 위 키부터 이어진
// 경로(예: watchTargets[0].option.run.interrupt)로 시작한다.
func WithKey(key string, err error) error {
	return fmt.Errorf("%s.%w", key, err)
}
