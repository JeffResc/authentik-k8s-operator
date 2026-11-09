package authentik

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type bindingsServer struct {
	mu       sync.Mutex
	groups   map[string]string // name -> pk
	bindings []map[string]any
	created  []map[string]any
	deleted  []string
}

func (s *bindingsServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	pagination := map[string]any{"next": 0, "previous": 0, "count": 0, "current": 1, "total_pages": 1, "start_index": 0, "end_index": 0}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v3/core/groups/":
		results := []map[string]any{}
		name := r.URL.Query().Get("name")
		if pk, ok := s.groups[name]; ok {
			results = append(results, map[string]any{"pk": pk, "num_pk": 1, "name": name, "parents_obj": []any{}, "roles_obj": []any{}, "inherited_roles_obj": []any{}, "children": []any{}, "children_obj": []any{}, "users_obj": []any{}})
		}
		json.NewEncoder(w).Encode(map[string]any{"pagination": pagination, "results": results, "autocomplete": map[string]any{}})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v3/policies/bindings/":
		json.NewEncoder(w).Encode(map[string]any{"pagination": pagination, "results": s.bindings, "autocomplete": map[string]any{}})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v3/policies/bindings/":
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		s.created = append(s.created, req)
		w.WriteHeader(http.StatusCreated)
		resp := map[string]any{"pk": "new", "target": req["target"], "group": req["group"], "order": req["order"], "policy_obj": nil, "group_obj": nil, "user_obj": nil, "expires": nil, "expiring": false}
		json.NewEncoder(w).Encode(resp)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v3/policies/bindings/"):
		s.deleted = append(s.deleted, strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v3/policies/bindings/"), "/"))
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func binding(pk, group string) map[string]any {
	b := map[string]any{"pk": pk, "target": "app-uid", "order": 0, "policy_obj": nil, "group_obj": nil, "user_obj": nil, "expires": nil, "expiring": false}
	if group != "" {
		b["group"] = group
	} else {
		b["policy"] = "policy-pk"
	}
	return b
}

func TestSyncGroupBindings(t *testing.T) {
	bs := &bindingsServer{
		groups: map[string]string{"Users": "g-users", "Admins": "g-admins"},
		bindings: []map[string]any{
			binding("b-users", "g-users"), // kept
			binding("b-stale", "g-stale"), // removed
			binding("b-policy", ""),       // policy binding, untouched
		},
	}
	server := httptest.NewServer(bs)
	defer server.Close()

	c := newTestClient(t, server)
	if err := c.SyncGroupBindings(context.Background(), "app-uid", []string{"Users", "Admins"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(bs.deleted) != 1 || bs.deleted[0] != "b-stale" {
		t.Errorf("expected only b-stale deleted, got %v", bs.deleted)
	}
	if len(bs.created) != 1 {
		t.Fatalf("expected one binding created, got %v", bs.created)
	}
	got := bs.created[0]
	if got["group"] != "g-admins" || got["target"] != "app-uid" || got["order"] != float64(1) {
		t.Errorf("unexpected created binding: %v", got)
	}
}

func TestSyncGroupBindings_GroupNotFound(t *testing.T) {
	bs := &bindingsServer{groups: map[string]string{}}
	server := httptest.NewServer(bs)
	defer server.Close()

	c := newTestClient(t, server)
	err := c.SyncGroupBindings(context.Background(), "app-uid", []string{"Missing"})
	if err == nil || !strings.Contains(err.Error(), `group "Missing" not found`) {
		t.Fatalf("expected group not found error, got %v", err)
	}
	if len(bs.created)+len(bs.deleted) != 0 {
		t.Errorf("expected no changes, got created=%v deleted=%v", bs.created, bs.deleted)
	}
}
