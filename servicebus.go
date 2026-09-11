package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// ServiceBusSender implementa MessageSender para Azure Service Bus — é o
// provider usado em produção (CLOUD_PROVIDER=azure), mesmo padrão validado
// no evaluation-service da Fase 3/4.
type ServiceBusSender struct {
	client    *azservicebus.Client
	queueName string
}

func NewServiceBusSender(connectionString, queueName string) (*ServiceBusSender, error) {
	client, err := azservicebus.NewClientFromConnectionString(connectionString, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar cliente Service Bus: %w", err)
	}
	return &ServiceBusSender{client: client, queueName: queueName}, nil
}

func (s *ServiceBusSender) SendEvent(ctx context.Context, d Donation) error {
	body, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("erro ao serializar evento: %w", err)
	}

	sender, err := s.client.NewSender(s.queueName, nil)
	if err != nil {
		return fmt.Errorf("erro ao criar sender: %w", err)
	}
	defer func() {
		if err := sender.Close(context.Background()); err != nil {
			log.Printf("Erro ao fechar sender do Service Bus: %v", err)
		}
	}()

	// Propaga o traceparent/tracestate como propriedades da mensagem, para
	// que o notification-service reconstrua o mesmo trace distribuído ao
	// consumir a mensagem (Distributed Tracing via mensageria assíncrona).
	appProps := make(map[string]interface{})
	for k, v := range injectTraceContext(ctx) {
		appProps[k] = v
	}

	err = sender.SendMessage(context.Background(), &azservicebus.Message{
		Body:                  body,
		ApplicationProperties: appProps,
	}, nil)
	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem para Service Bus: %w", err)
	}

	log.Printf("Evento de doação enviado para Service Bus (Donation ID: %d)", d.ID)
	return nil
}
