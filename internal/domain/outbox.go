package domain

type OutboxEvent struct {
	Topic   string
	Key     string
	Headers []byte
	Payload []byte
}
