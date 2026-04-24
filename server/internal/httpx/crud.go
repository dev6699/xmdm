package httpx

import (
	"context"
	"encoding/json"
	"net/http"

	"xmdm/server/internal/audit"
	"xmdm/server/internal/auth"
)

type Record interface {
	RecordID() string
	RecordStatus() string
}

type ResourceSpec[Req any, Resp Record] struct {
	Kind      string
	ReadPerm  auth.Permission
	WritePerm auth.Permission
	Decode    func(*http.Request) (Req, error)
	List      func(context.Context) ([]Resp, error)
	Create    func(context.Context, Req) (Resp, error)
	Update    func(context.Context, string, Req) (Resp, error)
	Retire    func(context.Context, string) (Resp, error)
	Audit     func(Resp) map[string]any
}

func RegisterCRUDFor[Req any, Resp Record](mux Router, svc *auth.Service, auditStore audit.Store, tenantID string, spec ResourceSpec[Req, Resp]) {
	collectionPath := "/" + spec.Kind
	itemPath := collectionPath + "/{id}"

	mux.HandleFunc(collectionPath, func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			if !auth.HasPermission(session.Permissions, spec.ReadPerm) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			items, err := spec.List(ctx)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, items)
		case http.MethodPost:
			if !auth.HasPermission(session.Permissions, spec.WritePerm) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			payload, err := spec.Decode(r)
			if err != nil {
				if err == ErrInvalidInput {
					http.Error(w, "invalid input", http.StatusBadRequest)
					return
				}
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			rec, err := spec.Create(ctx, payload)
			if err != nil {
				if err == ErrInvalidInput {
					http.Error(w, "invalid input", http.StatusBadRequest)
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if _, err := auditStore.Record(ctx, tenantID, session.Username, "create", spec.Kind, rec.RecordID(), spec.Audit(rec)); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, rec)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(itemPath, func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		session, ok := sessionFromRequest(r, svc)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		id := r.PathValue("id")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPatch:
			if !auth.HasPermission(session.Permissions, spec.WritePerm) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			payload, err := spec.Decode(r)
			if err != nil {
				if err == ErrInvalidInput {
					http.Error(w, "invalid input", http.StatusBadRequest)
					return
				}
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			rec, err := spec.Update(ctx, id, payload)
			if err != nil {
				if err == ErrInvalidInput {
					http.Error(w, "invalid input", http.StatusBadRequest)
					return
				}
				if err == ErrNotFound {
					http.NotFound(w, r)
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if _, err := auditStore.Record(ctx, tenantID, session.Username, "update", spec.Kind, rec.RecordID(), spec.Audit(rec)); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, rec)
		case http.MethodDelete:
			if !auth.HasPermission(session.Permissions, spec.WritePerm) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			rec, err := spec.Retire(ctx, id)
			if err != nil {
				if err == ErrNotFound {
					http.NotFound(w, r)
					return
				}
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			details := spec.Audit(rec)
			if details == nil {
				details = map[string]any{}
			}
			details["status"] = rec.RecordStatus()
			if _, err := auditStore.Record(ctx, tenantID, session.Username, "retire", spec.Kind, rec.RecordID(), details); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, rec)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func sessionFromRequest(r *http.Request, svc *auth.Service) (*auth.Session, bool) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return nil, false
	}
	session, ok := svc.Authenticate(cookie.Value)
	if !ok {
		return nil, false
	}
	return session, true
}

func DecodeJSONBody[T any](r *http.Request, dst *T) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
