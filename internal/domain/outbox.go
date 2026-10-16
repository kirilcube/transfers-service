package domain

type OutboxEvent struct {
	Topic   string
	Key     string
	Headers string
	Payload string
}
