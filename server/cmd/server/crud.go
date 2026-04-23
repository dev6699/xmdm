package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"xmdm/server/internal/admin"
	"xmdm/server/internal/auth"
)

type crudSpec struct {
	Kind      string
	ReadPerm  auth.Permission
	WritePerm auth.Permission
}

var adminCRUD = []crudSpec{
	{Kind: "users", ReadPerm: auth.PermissionAdminRead, WritePerm: auth.PermissionAdminWrite},
	{Kind: "roles", ReadPerm: auth.PermissionAdminRead, WritePerm: auth.PermissionAdminWrite},
	{Kind: "groups", ReadPerm: auth.PermissionAdminRead, WritePerm: auth.PermissionAdminWrite},
	{Kind: "policies", ReadPerm: auth.PermissionAdminRead, WritePerm: auth.PermissionAdminWrite},
	{Kind: "devices", ReadPerm: auth.PermissionDevicesRead, WritePerm: auth.PermissionDevicesWrite},
}

type crudPayload struct {
	Name  string         `json:"name"`
	Extra map[string]any `json:"extra"`
}

func registerCRUD(mux *http.ServeMux, svc *auth.Service, store *admin.Store, tenantID string) {
	for _, spec := range adminCRUD {
		spec := spec
		mux.HandleFunc("/admin/"+spec.Kind, func(w http.ResponseWriter, r *http.Request) {
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
				writeJSON(w, store.List(spec.Kind, tenantID))
			case http.MethodPost:
				if !auth.HasPermission(session.Permissions, spec.WritePerm) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				payload, err := decodeCRUDPayload(r)
				if err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				writeJSON(w, store.Create(spec.Kind, tenantID, payload.Name, payload.Extra))
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		})

		mux.HandleFunc("/admin/"+spec.Kind+"/", func(w http.ResponseWriter, r *http.Request) {
			session, ok := sessionFromRequest(r, svc)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			id := strings.TrimPrefix(r.URL.Path, "/admin/"+spec.Kind+"/")
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
				payload, err := decodeCRUDPayload(r)
				if err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				rec, err := store.Update(spec.Kind, tenantID, id, payload.Name, payload.Extra)
				if err != nil {
					http.NotFound(w, r)
					return
				}
				writeJSON(w, rec)
			case http.MethodDelete:
				if !auth.HasPermission(session.Permissions, spec.WritePerm) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				rec, err := store.Retire(spec.Kind, tenantID, id)
				if err != nil {
					http.NotFound(w, r)
					return
				}
				writeJSON(w, rec)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		})
	}
}

func decodeCRUDPayload(r *http.Request) (crudPayload, error) {
	var payload crudPayload
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		return crudPayload{}, err
	}
	return payload, nil
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
