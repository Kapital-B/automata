package http

import (
	"errors"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"
	"net/http"
)

func writeOperationError(w http.ResponseWriter, err error) {
	var failure *domainshared.OperationError
	var rule *domainmessages.MessageForwardRuleError
	status, message := http.StatusInternalServerError, "internal"
	if errors.As(err, &failure) {
		message = failure.Message
		switch failure.Kind {
		case "invalid":
			status = http.StatusBadRequest
		case "unprocessable":
			status = http.StatusUnprocessableEntity
		case "missing":
			status = http.StatusNotFound
		case "unavailable":
			status = http.StatusServiceUnavailable
		}
	} else if errors.As(err, &rule) {
		status, message = http.StatusUnprocessableEntity, rule.Reason
	}
	writeJSON(w, status, map[string]string{"error": message})
}
