package config

import "testing"

func TestConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/support")
	for _, name := range []string{"PORT", "METRICS_PORT", "LOG_LEVEL", "DB_POOL_SIZE", "QUERY_TIMEOUT", "SHUTDOWN_TIMEOUT", "SESSION_DURATION", "SECURE_COOKIE"} {
		t.Setenv(name, "")
	}

	c, err := Load()
	if err != nil || c.Port != "8080" || c.MetricsPort != "9090" || c.PoolSize != 20 || c.SessionDuration.Hours() != 24 {
		t.Fatalf("defaults: %+v %v", c, err)
	}

	for name, value := range map[string]string{"PORT": "0", "METRICS_PORT": "65536", "LOG_LEVEL": "verbose", "DB_POOL_SIZE": "-1", "QUERY_TIMEOUT": "0s", "SHUTDOWN_TIMEOUT": "invalid", "SESSION_DURATION": "-1h", "SECURE_COOKIE": "maybe"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			if _, err := Load(); err == nil {
				t.Fatalf("accepted %s=%s", name, value)
			}
		})
	}
	for _, value := range []string{"0", "invalid", "8080", "08080"} {
		t.Run("metrics-port-"+value, func(t *testing.T) {
			t.Setenv("METRICS_PORT", value)
			if _, err := Load(); err == nil {
				t.Fatalf("accepted METRICS_PORT=%s", value)
			}
		})
	}

	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing database URL accepted")
	}
}

func TestLabFaultConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/support")
	t.Setenv("LAB_FAULT_RATE", "0.2")
	t.Setenv("LAB_FAULT_DELAY", "1.5s")
	c, err := Load()
	if err != nil || c.LabFaultRate != 0.2 || c.LabFaultDelay.Milliseconds() != 1500 {
		t.Fatalf("fault config: %+v %v", c, err)
	}
	for _, value := range []string{"-0.1", "1.1", "NaN", "Inf", "bad"} {
		t.Run("rate-"+value, func(t *testing.T) {
			t.Setenv("LAB_FAULT_RATE", value)
			if _, err := Load(); err == nil {
				t.Fatalf("accepted rate %s", value)
			}
		})
	}
	for _, value := range []string{"-1s", "bad"} {
		t.Run("delay-"+value, func(t *testing.T) {
			t.Setenv("LAB_FAULT_DELAY", value)
			if _, err := Load(); err == nil {
				t.Fatalf("accepted delay %s", value)
			}
		})
	}
}
