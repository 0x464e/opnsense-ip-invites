package httpapi

import (
	"errors"
	"github.com/0x464e/opnsense-ip-invites/internal/service"
	"github.com/0x464e/opnsense-ip-invites/internal/store"
	"net/http"
)

func (s *Server) listError(w http.ResponseWriter, err error) {
	if errors.Is(err, service.ErrInput) {
		problem(w, 400, "invalid_list", err.Error())
	} else if errors.Is(err, store.ErrConflict) {
		problem(w, 409, "list_conflict", err.Error())
	} else {
		s.fail(w, err)
	}
}
func (s *Server) createList(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Alias       string `json:"alias"`
	}
	if !body(w, r, &in) {
		return
	}
	l, err := s.svc.CreateList(r.Context(), store.List{Name: in.Name, Description: in.Description, Alias: in.Alias})
	if err != nil && !errors.Is(err, service.ErrSync) {
		s.listError(w, err)
		return
	}
	code, status := 201, "synced"
	message := "List created."
	if err != nil {
		code, status = 202, "pending"
		message = "List saved. Firewall synchronization is pending; check its status."
	}
	write(w, code, map[string]any{"list": l, "status": status, "message": message})
}
func (s *Server) updateList(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Alias       string `json:"alias"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.svc.UpdateList(r.Context(), store.List{ID: r.PathValue("id"), Name: in.Name, Description: in.Description, Alias: in.Alias}); err != nil {
		s.listError(w, err)
		return
	}
	write(w, 200, map[string]string{"status": "saved"})
}
func (s *Server) deleteList(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !body(w, r, &in) {
		return
	}
	s.mutationResult(w, s.svc.DeleteList(r.Context(), r.PathValue("id")))
}
