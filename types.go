package main

import (
	"database/sql"
	"time"
)

// Donation representa uma doação processada pelo serviço.
type Donation struct {
	ID        int       `json:"id"`
	NgoID     int       `json:"ngo_id"`
	Amount    float64   `json:"amount"`
	DonorName string    `json:"donor_name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// App carrega as dependências injetadas nos handlers.
type App struct {
	DB        *sql.DB
	MsgSender MessageSender
}
