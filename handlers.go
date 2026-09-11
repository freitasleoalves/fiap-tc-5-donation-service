package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

func (a *App) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": serviceName}); err != nil {
		log.Printf("Erro ao encodar resposta health: %v", err)
	}
}

func (a *App) DonationHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost {
		var d Donation
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			http.Error(w, `{"error":"Payload inválido"}`, http.StatusBadRequest)
			return
		}

		d.Status = "APPROVED" // Simulação de gateway de pagamento
		err := a.DB.QueryRow(
			"INSERT INTO donations (ngo_id, amount, donor_name, status) VALUES ($1, $2, $3, $4) RETURNING id, created_at",
			d.NgoID, d.Amount, d.DonorName, d.Status,
		).Scan(&d.ID, &d.CreatedAt)

		if err != nil {
			log.Printf("Erro ao salvar doação: %v", err)
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}

		if a.MsgSender != nil {
			// O SpanContext da requisição é preservado num novo
			// context.Background() (desvinculado do ciclo de vida da
			// requisição HTTP, que é cancelado assim que o handler
			// retorna) para que o span de envio da mensagem continue
			// fazendo parte do mesmo trace distribuído no APM.
			msgCtx := trace.ContextWithSpanContext(context.Background(), trace.SpanContextFromContext(r.Context()))
			donationCopy := d
			go func() {
				if err := a.MsgSender.SendEvent(msgCtx, donationCopy); err != nil {
					log.Printf("Erro ao despachar evento de notificação: %v", err)
				}
			}()
		} else {
			log.Printf("[QUEUE_DISABLED] Doação %d (NGO %d, R$ %.2f) não notificada", d.ID, d.NgoID, d.Amount)
		}

		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(d); err != nil {
			log.Printf("Erro ao encodar resposta da doação: %v", err)
		}
		return
	}

	if r.Method == http.MethodGet {
		rows, err := a.DB.Query("SELECT id, ngo_id, amount, donor_name, status, created_at FROM donations ORDER BY id DESC")
		if err != nil {
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}
		defer func() {
			if err := rows.Close(); err != nil {
				log.Printf("Erro ao fechar rows: %v", err)
			}
		}()

		donations := []Donation{}
		for rows.Next() {
			var d Donation
			if err := rows.Scan(&d.ID, &d.NgoID, &d.Amount, &d.DonorName, &d.Status, &d.CreatedAt); err != nil {
				log.Printf("Erro ao ler linha de doação: %v", err)
				continue
			}
			donations = append(donations, d)
		}

		if err := json.NewEncoder(w).Encode(donations); err != nil {
			log.Printf("Erro ao encodar lista de doações: %v", err)
		}
		return
	}

	http.Error(w, `{"error":"Método não permitido"}`, http.StatusMethodNotAllowed)
}
