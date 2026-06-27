package config

import "testing"

func TestDSN_EscapesSpecialChars(t *testing.T) {
	db := DBConfig{
		Host: "localhost", Port: 5432, User: "post@gres",
		Password: "p@ss:w/rd?#", Name: "app", SSLMode: "disable",
	}
	// Special chars in credentials must be percent-encoded, not break the URL.
	got := db.DSN()
	want := "postgres://post%40gres:p%40ss%3Aw%2Frd%3F%23@localhost:5432/app?sslmode=disable"
	if got != want {
		t.Fatalf("DSN()\n got: %s\nwant: %s", got, want)
	}
}

func TestValidate_RejectsWeakSecretInProd(t *testing.T) {
	base := func(secret string) *Config {
		return &Config{Env: "production", JWT: JWTConfig{Secret: secret}, DB: DBConfig{Host: "db"}}
	}
	// placeholder / short secrets must fail in production
	for _, s := range []string{"", "change-me", "change-me-in-production", "short"} {
		if err := base(s).validate(); err == nil {
			t.Errorf("expected error for prod secret %q, got nil", s)
		}
	}
	// a strong 32+ byte secret passes
	if err := base("a-really-long-and-random-secret-value-123").validate(); err != nil {
		t.Errorf("expected strong secret to pass, got %v", err)
	}
	// dev env is lenient
	dev := &Config{Env: "development", JWT: JWTConfig{Secret: "change-me"}, DB: DBConfig{Host: "db"}}
	if err := dev.validate(); err != nil {
		t.Errorf("dev env should not enforce secret strength, got %v", err)
	}
}
