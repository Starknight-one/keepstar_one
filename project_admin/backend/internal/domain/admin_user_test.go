package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAdminUser_HasPassword_JSON_True(t *testing.T) {
	u := AdminUser{ID: "u1", Email: "a@b.com", PasswordHash: "$2a$10$hash"}
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"has_password":true`) {
		t.Errorf("missing has_password=true; got %s", s)
	}
	if strings.Contains(s, `"$2a$10$hash"`) {
		t.Errorf("PasswordHash leaked into JSON: %s", s)
	}
}

func TestAdminUser_HasPassword_JSON_False(t *testing.T) {
	u := AdminUser{ID: "u1", Email: "a@b.com"} // no PasswordHash
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"has_password":false`) {
		t.Errorf("expected has_password=false; got %s", s)
	}
}

func TestAdminUser_HasPasswordHelper(t *testing.T) {
	empty := AdminUser{}
	if empty.HasPassword() {
		t.Error("empty user should report no password")
	}
	pw := AdminUser{PasswordHash: "h"}
	if !pw.HasPassword() {
		t.Error("user with hash should report HasPassword=true")
	}
	var nilU *AdminUser
	if nilU.HasPassword() {
		t.Error("nil receiver should report false")
	}
}
