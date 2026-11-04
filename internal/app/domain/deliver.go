package domain

// Deliver는 hookBatch가 한 번 기다리는 동안 모은 이벤트를 어떤 실행으로 넘길지 정한다.
type Deliver int

const (
	// DeliverLast는 최종 변화가 있는 경로 가운데 가장 나중에 바뀐 경로 하나로 실행 하나를 만든다.
	DeliverLast Deliver = iota
	// DeliverLastPerPath는 최종 변화가 있는 경로마다 실행 하나를 만들고, 경로가 마지막으로 바뀐 순서대로 둔다.
	DeliverLastPerPath
	// DeliverEach는 이벤트마다 실행 하나를 만들고, 이벤트가 생긴 순서대로 둔다.
	DeliverEach
)
