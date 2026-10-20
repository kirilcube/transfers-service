package domain

type OutboxEvent struct {
	Topic   string
	Key     string
	Type    string
	Headers []byte
	Payload []byte
}
