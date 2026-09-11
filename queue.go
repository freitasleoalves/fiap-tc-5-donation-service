package main

import "context"

// MessageSender abstrai o envio do evento de doação para a fila. O ctx
// carrega o SpanContext da requisição HTTP original, permitindo que o span
// de mensageria fique vinculado ao mesmo trace distribuído no APM (o
// notification-service reconstrói esse trace ao consumir a mensagem).
type MessageSender interface {
	SendEvent(ctx context.Context, d Donation) error
}
