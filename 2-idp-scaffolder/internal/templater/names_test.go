package templater

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckName(t *testing.T) {
	valid := []string{"a", "app-a", "tenant-a", "team-a", "dev", "a1", "a" + strings.Repeat("b", 39)}
	for _, v := range valid {
		if err := checkName("app-name", v); err != nil {
			t.Errorf("checkName(%q) = %v, want nil", v, err)
		}
	}
	invalid := []string{
		"", "App-A", "-app", "app-", "1app", "app_a", "app.a", "app a",
		"Bad Name: x", "../../x", "../prod", "a/b", "a" + strings.Repeat("b", 40),
	}
	for _, v := range invalid {
		err := checkName("app-name", v)
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("checkName(%q) = %v, want ErrInvalidName", v, err)
		}
	}
}

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error %v is not a *ValidationError", err)
	}
	return ve.Field
}

func TestValidateTenantNames(t *testing.T) {
	if err := validateTenantNames(Config{TenantName: "tenant-a", Owners: []string{"team-a"}}); err != nil {
		t.Errorf("good config: %v", err)
	}
	err := validateTenantNames(Config{TenantName: "tenant-a", Owners: []string{"team-a", "Team B"}})
	if !errors.Is(err, ErrInvalidName) || fieldOf(t, err) != "owner" {
		t.Errorf("bad owner: got %v, want ErrInvalidName on field owner", err)
	}
}

func TestValidateServiceNames(t *testing.T) {
	good := Config{TenantName: "tenant-a", AppName: "app-a", Env: "dev"}
	if err := validateServiceNames(good); err != nil {
		t.Errorf("empty system must be allowed: %v", err)
	}
	bad := good
	bad.SystemName = "Sys A"
	if err := validateServiceNames(bad); !errors.Is(err, ErrInvalidName) || fieldOf(t, err) != "system" {
		t.Errorf("bad system: got %v", err)
	}
	bad = good
	bad.Env = "../prod"
	if err := validateServiceNames(bad); !errors.Is(err, ErrInvalidName) || fieldOf(t, err) != "env" {
		t.Errorf("bad env: got %v", err)
	}
}

func TestValidateServiceNamesJoinedLength(t *testing.T) {
	a := func(n int) string { return strings.Repeat("a", n) }
	pass := []Config{
		{TenantName: "tenant-a", AppName: "app-a", Env: "dev"},
		{TenantName: a(20), AppName: a(20), Env: a(21)}, // joined 63
	}
	for _, c := range pass {
		if err := validateServiceNames(c); err != nil {
			t.Errorf("%d+%d+%d: %v, want nil", len(c.TenantName), len(c.AppName), len(c.Env), err)
		}
	}
	fail := []Config{
		{TenantName: a(20), AppName: a(20), Env: a(22)}, // joined 64
		{TenantName: a(40), AppName: a(40), Env: a(40)},
	}
	for _, c := range fail {
		err := validateServiceNames(c)
		if !errors.Is(err, ErrNameTooLong) || fieldOf(t, err) != "tenant-name+app-name+env" {
			t.Errorf("%d+%d+%d: got %v, want ErrNameTooLong", len(c.TenantName), len(c.AppName), len(c.Env), err)
		}
	}
}
