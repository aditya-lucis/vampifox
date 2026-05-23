package den

import (
	"context"
	"testing"
	"time"
)

// ── Mock Module untuk testing ─────────────────────────────────────

type mockModule struct {
	name       string
	version    string
	dependsOn  []string
	bootErr    error
	shutdownErr error
	booted     bool
	shutdown   bool
}

func (m *mockModule) Name() string       { return m.name }
func (m *mockModule) Version() string    { return m.version }
func (m *mockModule) DependsOn() []string { return m.dependsOn }
func (m *mockModule) Boot(_ context.Context, _ *Den) error {
	if m.bootErr != nil {
		return m.bootErr
	}
	m.booted = true
	return nil
}
func (m *mockModule) Shutdown(_ context.Context) error {
	m.shutdown = true
	return m.shutdownErr
}

// newTestDen membuat Den minimal untuk testing tanpa file config.
func newTestDen(t *testing.T) *Den {
	t.Helper()
	cfg := &VampConfig{
		App:    AppConfig{Name: "TestFox", Env: "development", Timezone: "Asia/Jakarta"},
		Server: ServerConfig{Host: "127.0.0.1", Port: 0, ShutdownTimeout: 5 * time.Second},
		Fangs:  FangsConfig{Driver: DBDriverSQLite, SQLitePath: ":memory:"},
		Log:    LogConfig{Level: "error", Format: "console", Output: "stdout"},
		Sanctum: SanctumConfig{
			AccessSecret:  "test-access-secret-32-characters!",
			RefreshSecret: "test-refresh-secret-32-characters!",
		},
	}

	logger, err := buildLogger(cfg.Log)
	if err != nil {
		t.Fatalf("buildLogger gagal: %v", err)
	}

	return &Den{
		cfg:    cfg,
		logger: logger,
		modMap: make(map[string]Module),
	}
}

// ── Tests ─────────────────────────────────────────────────────────

func TestDen_RegisterModules_Success(t *testing.T) {
	d := newTestDen(t)

	m1 := &mockModule{name: "alpha", version: "1.0.0"}
	m2 := &mockModule{name: "beta", version: "1.0.0", dependsOn: []string{"alpha"}}

	if err := d.RegisterModules(m1, m2); err != nil {
		t.Fatalf("RegisterModules gagal: %v", err)
	}

	if len(d.modules) != 2 {
		t.Errorf("len(modules) = %d, want 2", len(d.modules))
	}

	// Urutan harus dipertahankan
	if d.modules[0].Name() != "alpha" {
		t.Errorf("modules[0] = %q, want alpha", d.modules[0].Name())
	}
	if d.modules[1].Name() != "beta" {
		t.Errorf("modules[1] = %q, want beta", d.modules[1].Name())
	}
}

func TestDen_RegisterModules_DuplicateName(t *testing.T) {
	d := newTestDen(t)

	m1 := &mockModule{name: "alpha", version: "1.0.0"}
	m2 := &mockModule{name: "alpha", version: "2.0.0"} // nama sama!

	err := d.RegisterModules(m1, m2)
	if err == nil {
		t.Error("RegisterModules harus error untuk nama duplikat")
	}
}

func TestDen_RegisterModules_EmptyName(t *testing.T) {
	d := newTestDen(t)

	m := &mockModule{name: "", version: "1.0.0"}
	err := d.RegisterModules(m)
	if err == nil {
		t.Error("RegisterModules harus error untuk nama kosong")
	}
}

func TestDen_ResolveDependencies_Success(t *testing.T) {
	d := newTestDen(t)

	m1 := &mockModule{name: "core", version: "1.0.0"}
	m2 := &mockModule{name: "accounting", version: "1.0.0", dependsOn: []string{"core"}}

	_ = d.RegisterModules(m1, m2)

	if err := d.resolveDependencies(); err != nil {
		t.Errorf("resolveDependencies harus sukses: %v", err)
	}
}

func TestDen_ResolveDependencies_MissingDep(t *testing.T) {
	d := newTestDen(t)

	// accounting butuh inventory, tapi inventory tidak didaftarkan
	m := &mockModule{name: "accounting", version: "1.0.0", dependsOn: []string{"inventory"}}
	_ = d.RegisterModules(m)

	err := d.resolveDependencies()
	if err == nil {
		t.Error("resolveDependencies harus error jika dependency tidak ada")
	}
}

func TestDen_Module_Lookup(t *testing.T) {
	d := newTestDen(t)

	m := &mockModule{name: "inventory", version: "1.0.0"}
	_ = d.RegisterModules(m)

	found, ok := d.Module("inventory")
	if !ok {
		t.Error("Module('inventory') harus ketemu")
	}
	if found.Name() != "inventory" {
		t.Errorf("found.Name() = %q, want inventory", found.Name())
	}

	_, ok = d.Module("tidak-ada")
	if ok {
		t.Error("Module('tidak-ada') harus tidak ketemu")
	}
}

func TestDen_Config_Accessor(t *testing.T) {
	d := newTestDen(t)

	cfg := d.Config()
	if cfg == nil {
		t.Fatal("Config() tidak boleh nil")
	}
	if cfg.App.Name != "TestFox" {
		t.Errorf("cfg.App.Name = %q, want TestFox", cfg.App.Name)
	}
}

func TestDen_Logger_Accessor(t *testing.T) {
	d := newTestDen(t)

	log := d.Logger()
	if log == nil {
		t.Fatal("Logger() tidak boleh nil")
	}
}

func TestFangsConfig_Validate_AllDrivers(t *testing.T) {
	validCases := []FangsConfig{
		{Driver: DBDriverPostgres, Host: "localhost", User: "u", DBName: "db"},
		{Driver: DBDriverMySQL, Host: "localhost", User: "u", DBName: "db"},
		{Driver: DBDriverSQLServer, Host: "localhost", User: "u", DBName: "db"},
		{Driver: DBDriverSQLite, SQLitePath: ":memory:"},
	}

	for _, cfg := range validCases {
		if err := cfg.Validate(); err != nil {
			t.Errorf("driver %q harus valid: %v", cfg.Driver, err)
		}
	}
}