package config

import "testing"

func TestConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/support")
	for _, name := range []string{"PORT", "LOG_LEVEL", "DB_POOL_SIZE", "QUERY_TIMEOUT", "SHUTDOWN_TIMEOUT", "SESSION_DURATION", "SECURE_COOKIE"} {
		t.Setenv(name, "")
	}

	c, err := Load()
	if err != nil || c.Port != "8080" || c.PoolSize != 20 || c.SessionDuration.Hours() != 24 {
		t.Fatalf("defaults: %+v %v", c, err)
	}

	for name, value := range map[string]string{"PORT": "0", "LOG_LEVEL": "verbose", "DB_POOL_SIZE": "-1", "QUERY_TIMEOUT": "0s", "SHUTDOWN_TIMEOUT": "invalid", "SESSION_DURATION": "-1h", "SECURE_COOKIE": "maybe"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if _, err := Load(); err == nil {
				t.Fatalf("accepted %s=%s", name, value)
			}
		})
	}

	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing database URL accepted")
	}
}
