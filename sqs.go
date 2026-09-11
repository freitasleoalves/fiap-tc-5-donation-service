package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
)

// SQSSender implementa MessageSender para AWS SQS — usado apenas em
// desenvolvimento local (LocalStack). Em produção o ambiente usa Azure
// Service Bus (ver servicebus.go), selecionado via CLOUD_PROVIDER.
type SQSSender struct {
	SqsSvc   *sqs.SQS
	QueueURL string
}

func (s *SQSSender) SendEvent(ctx context.Context, d Donation) error {
	body, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("erro ao serializar evento SQS: %w", err)
	}

	// Propaga o traceparent/tracestate como Message Attributes, para o
	// notification-service reconstruir o mesmo trace distribuído.
	msgAttrs := make(map[string]*sqs.MessageAttributeValue)
	for k, v := range injectTraceContext(ctx) {
		msgAttrs[k] = &sqs.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(v),
		}
	}

	_, err = s.SqsSvc.SendMessage(&sqs.SendMessageInput{
		MessageBody:       aws.String(string(body)),
		QueueUrl:          aws.String(s.QueueURL),
		MessageAttributes: msgAttrs,
	})
	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem para SQS: %w", err)
	}

	log.Printf("Evento de doação enviado para SQS (Donation ID: %d)", d.ID)
	return nil
}
