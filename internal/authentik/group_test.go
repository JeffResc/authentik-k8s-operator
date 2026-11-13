package authentik

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "goauthentik.io/api/v3"
)

const (
	testGroupUID  = "11111111-1111-1111-1111-111111111111"
	testParentUID = "22222222-2222-2222-2222-222222222222"
	testRoleUID   = "33333333-3333-3333-3333-333333333333"
)

func testGroup(uid, name string) api.Group {
	// The SDK requires these keys on unmarshal and drops nil slices on marshal.
	return api.Group{
		Pk:                uid,
		Name:              name,
		ParentsObj:        []api.RelatedGroup{},
		UsersObj:          []api.PartialUser{},
		RolesObj:          []api.Role{},
		InheritedRolesObj: []api.Role{},
		Children:          []string{},
		ChildrenObj:       []api.RelatedGroup{},
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Errorf("failed to encode response: %v", err)
	}
}

// groupServer serves group and role lookups. It records the JSON body of the
// last create (POST) or update (PATCH) request in lastBody.
type groupServer struct {
	t        *testing.T
	groups   map[string]string // name -> uid
	roles    map[string]string // name -> uid
	lastBody map[string]interface{}
	method   string
}

func (s *groupServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/v3/core/groups/" && r.Method == http.MethodGet:
		var results []api.Group
		if uid, ok := s.groups[r.URL.Query().Get("name")]; ok {
			results = append(results, testGroup(uid, r.URL.Query().Get("name")))
		}
		writeJSON(s.t, w, api.PaginatedGroupList{Results: results})
	case r.URL.Path == "/api/v3/rbac/roles/" && r.Method == http.MethodGet:
		var results []api.Role
		if uid, ok := s.roles[r.URL.Query().Get("name")]; ok {
			results = append(results, api.Role{Pk: uid, Name: r.URL.Query().Get("name")})
		}
		writeJSON(s.t, w, api.PaginatedRoleList{Results: results})
	case r.Method == http.MethodPost || r.Method == http.MethodPatch:
		s.method = r.Method
		body, _ := io.ReadAll(r.Body)
		s.lastBody = map[string]interface{}{}
		if err := json.Unmarshal(body, &s.lastBody); err != nil {
			s.t.Errorf("invalid request body: %v", err)
		}
		name, _ := s.lastBody["name"].(string)
		writeJSON(s.t, w, testGroup(testGroupUID, name))
	default:
		http.NotFound(w, r)
	}
}

func TestGetGroupByUID_Found(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/core/groups/"+testGroupUID+"/" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		writeJSON(t, w, testGroup(testGroupUID, "admins"))
	}))
	defer server.Close()

	info, err := newTestClient(t, server).GetGroupByUID(context.Background(), testGroupUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil || info.UID != testGroupUID || info.Name != "admins" {
		t.Errorf("unexpected group info: %+v", info)
	}
}

func TestGetGroupByUID_NotFound(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	info, err := newTestClient(t, server).GetGroupByUID(context.Background(), testGroupUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil, got %+v", info)
	}
}

func TestGetGroupByName(t *testing.T) {
	gs := &groupServer{t: t, groups: map[string]string{"admins": testGroupUID}}
	server := httptest.NewServer(gs)
	defer server.Close()
	c := newTestClient(t, server)

	info, err := c.GetGroupByName(context.Background(), "admins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil || info.UID != testGroupUID {
		t.Errorf("unexpected group info: %+v", info)
	}

	info, err = c.GetGroupByName(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil, got %+v", info)
	}
}

func TestCreateGroup_ResolvesReferences(t *testing.T) {
	gs := &groupServer{
		t:      t,
		groups: map[string]string{"parent": testParentUID},
		roles:  map[string]string{"editor": testRoleUID},
	}
	server := httptest.NewServer(gs)
	defer server.Close()

	uuidParent := "44444444-4444-4444-4444-444444444444"
	info, err := newTestClient(t, server).CreateGroup(context.Background(), &GroupOptions{
		Name:        "child",
		IsSuperuser: true,
		Parents:     []string{"parent", uuidParent},
		Roles:       []string{"editor"},
		Attributes:  map[string]interface{}{"team": "platform"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.UID != testGroupUID || info.Name != "child" {
		t.Errorf("unexpected group info: %+v", info)
	}
	if gs.method != http.MethodPost {
		t.Errorf("expected POST, got %s", gs.method)
	}

	assertStringList(t, gs.lastBody["parents"], []string{testParentUID, uuidParent})
	assertStringList(t, gs.lastBody["roles"], []string{testRoleUID})
	if gs.lastBody["is_superuser"] != true {
		t.Errorf("expected is_superuser=true, got %v", gs.lastBody["is_superuser"])
	}
	if attrs, _ := gs.lastBody["attributes"].(map[string]interface{}); attrs["team"] != "platform" {
		t.Errorf("unexpected attributes: %v", gs.lastBody["attributes"])
	}
	if _, ok := gs.lastBody["users"]; ok {
		t.Error("create must not send users")
	}
}

func TestUpdateGroup_EmptyFieldsClearAndUsersOmitted(t *testing.T) {
	gs := &groupServer{t: t}
	server := httptest.NewServer(gs)
	defer server.Close()

	if _, err := newTestClient(t, server).UpdateGroup(context.Background(), testGroupUID, &GroupOptions{Name: "g"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gs.method != http.MethodPatch {
		t.Errorf("expected PATCH, got %s", gs.method)
	}

	// Empty spec values must be sent so they clear existing values in Authentik.
	assertStringList(t, gs.lastBody["parents"], []string{})
	assertStringList(t, gs.lastBody["roles"], []string{})
	if attrs, ok := gs.lastBody["attributes"].(map[string]interface{}); !ok || len(attrs) != 0 {
		t.Errorf("expected empty attributes object, got %v", gs.lastBody["attributes"])
	}
	if gs.lastBody["is_superuser"] != false {
		t.Errorf("expected is_superuser=false, got %v", gs.lastBody["is_superuser"])
	}
	if _, ok := gs.lastBody["users"]; ok {
		t.Error("update must not send users")
	}
}

func TestCreateGroup_MissingReferences(t *testing.T) {
	gs := &groupServer{t: t}
	server := httptest.NewServer(gs)
	defer server.Close()
	c := newTestClient(t, server)

	_, err := c.CreateGroup(context.Background(), &GroupOptions{Name: "g", Parents: []string{"nope"}})
	if err == nil || !strings.Contains(err.Error(), `parent group "nope" not found`) {
		t.Errorf("expected parent not found error, got %v", err)
	}

	_, err = c.CreateGroup(context.Background(), &GroupOptions{Name: "g", Roles: []string{"nope"}})
	if err == nil || !strings.Contains(err.Error(), `role "nope" not found`) {
		t.Errorf("expected role not found error, got %v", err)
	}

	if gs.method != "" {
		t.Error("expected no create request when references are missing")
	}
}

func TestCreateGroup_RequiresName(t *testing.T) {
	c := &APIClient{}
	if _, err := c.CreateGroup(context.Background(), &GroupOptions{}); err == nil {
		t.Error("expected error for empty name")
	}
	if _, err := c.CreateGroup(context.Background(), nil); err == nil {
		t.Error("expected error for nil options")
	}
}

func TestDeleteGroup(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := newTestClient(t, server).DeleteGroup(context.Background(), testGroupUID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v3/core/groups/"+testGroupUID+"/" {
		t.Errorf("unexpected request %s %s", gotMethod, gotPath)
	}
}

func assertStringList(t *testing.T, got interface{}, want []string) {
	t.Helper()
	list, ok := got.([]interface{})
	if !ok {
		t.Errorf("expected list %v, got %v (%T)", want, got, got)
		return
	}
	if len(list) != len(want) {
		t.Errorf("expected %v, got %v", want, list)
		return
	}
	for i := range want {
		if list[i] != want[i] {
			t.Errorf("expected %v, got %v", want, list)
			return
		}
	}
}

func TestGetGroupByUID_ReturnsOwner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g := testGroup(testGroupUID, "g")
		g.Attributes = map[string]interface{}{GroupOwnerAttribute: "ns/g"}
		writeJSON(t, w, g)
	}))
	defer server.Close()

	info, err := newTestClient(t, server).GetGroupByUID(context.Background(), testGroupUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil || info.Owner != "ns/g" {
		t.Errorf("expected owner ns/g, got %+v", info)
	}
}

func TestUpdateGroup_WritesOwnerMarker(t *testing.T) {
	gs := &groupServer{t: t}
	server := httptest.NewServer(gs)
	defer server.Close()

	// A spec attribute with the marker key must not override the real owner.
	attrs := map[string]interface{}{"team": "platform", GroupOwnerAttribute: "other/forged"}
	if _, err := newTestClient(t, server).UpdateGroup(context.Background(), testGroupUID, &GroupOptions{
		Name: "g", Owner: "ns/g", Attributes: attrs,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sent, _ := gs.lastBody["attributes"].(map[string]interface{})
	if sent[GroupOwnerAttribute] != "ns/g" || sent["team"] != "platform" {
		t.Errorf("expected owner marker ns/g and team attribute, got %v", gs.lastBody["attributes"])
	}
	if attrs[GroupOwnerAttribute] != "other/forged" {
		t.Error("caller attributes map must not be mutated")
	}
}
