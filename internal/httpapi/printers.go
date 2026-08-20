package httpapi

import (
	"net/http"
)

type printerInfoResponse struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
}

func (r router) listPrinters(w http.ResponseWriter, request *http.Request) {
	if r.dependencies.Printers == nil {
		dependencyUnavailable(w, "printer adapter is unavailable")
		return
	}
	printers, err := r.dependencies.Printers.List(request.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "PRINTER_LIST_FAILED", "unable to list printers")
		return
	}
	response := make([]printerInfoResponse, len(printers))
	for index, item := range printers {
		response[index] = printerInfoResponse{Name: item.Name, IsDefault: item.IsDefault}
	}
	writeAPISuccess(w, http.StatusOK, response)
}
