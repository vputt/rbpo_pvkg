package httpapi

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

const (
	codeInvalidRequest     = "invalid_request"
	codeInvalidCredentials = "invalid_credentials"
	codeInternalError      = "internal_error"
)

const messageInternalError = "внутренняя ошибка"

type errorEnvelope struct {
	Error errorDetails `json:"error"`
}

type errorDetails struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError отдаёт тело ошибки в одном виде на весь API. Подробности сюда не попадают:
// ни SQL, ни имена таблиц, ни пути, ни стек.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: errorDetails{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("ответ не сформирован: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"error":{"code":"internal_error","message":"внутренняя ошибка"}}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}
