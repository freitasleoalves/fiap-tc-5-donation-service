package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	_ "github.com/jackc/pgx/v4/stdlib"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	ctx := context.Background()

	// --- OpenTelemetry (traces + métricas via OTel Collector) ---
	shutdownOTel, err := initOTel(ctx)
	if err != nil {
		log.Fatalf("Não foi possível inicializar o OpenTelemetry: %v", err)
	}
	defer func() {
		if err := shutdownOTel(ctx); err != nil {
			log.Printf("Erro ao encerrar o OpenTelemetry: %v", err)
		}
	}()

	// --- Configuração ---
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL é obrigatória")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("Erro ao abrir conexão com o banco de dados: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("Erro ao conectar ao banco de dados: %v", err)
	}
	log.Println("Conectado ao PostgreSQL (donation-service).")

	// --- Mensageria: Azure Service Bus (produção) ou AWS SQS (dev local) ---
	msgSender := buildMessageSender()

	app := &App{DB: db, MsgSender: msgSender}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", app.HealthHandler)
	mux.HandleFunc("/donations", app.DonationHandler)

	log.Printf("donation-service rodando na porta %s", port)
	log.Fatal(http.ListenAndServe(":"+port, instrumentHandler(mux)))
}

// buildMessageSender seleciona o provider de mensageria com base em
// CLOUD_PROVIDER ("azure" ou "aws", default "aws" para manter o
// comportamento original do código-fonte em dev local com LocalStack).
func buildMessageSender() MessageSender {
	cloudProvider := os.Getenv("CLOUD_PROVIDER")

	if cloudProvider == "azure" {
		connStr := os.Getenv("AZURE_SERVICEBUS_CONNECTION_STRING")
		queueName := os.Getenv("AZURE_SERVICEBUS_QUEUE_NAME")
		if connStr == "" || queueName == "" {
			log.Fatal("AZURE_SERVICEBUS_CONNECTION_STRING e AZURE_SERVICEBUS_QUEUE_NAME devem ser definidos para CLOUD_PROVIDER=azure")
		}
		sender, err := NewServiceBusSender(connStr, queueName)
		if err != nil {
			log.Fatalf("Não foi possível criar Service Bus sender: %v", err)
		}
		log.Println("Cliente Azure Service Bus inicializado com sucesso.")
		return sender
	}

	queueURL := os.Getenv("AWS_SQS_URL")
	region := os.Getenv("AWS_REGION")
	if queueURL == "" || region == "" {
		log.Println("Atenção: mensageria desabilitada (AWS_SQS_URL/AWS_REGION não definidos e CLOUD_PROVIDER != azure).")
		return nil
	}

	awsCfg := &aws.Config{Region: aws.String(region)}
	if endpoint := os.Getenv("AWS_ENDPOINT_URL"); endpoint != "" {
		awsCfg.Endpoint = aws.String(endpoint)
		awsCfg.S3ForcePathStyle = aws.Bool(true)
	}
	sess, err := session.NewSession(awsCfg)
	if err != nil {
		log.Fatalf("Não foi possível criar sessão AWS: %v", err)
	}
	log.Println("Integração com AWS SQS ativada.")
	return &SQSSender{SqsSvc: sqs.New(sess), QueueURL: queueURL}
}
