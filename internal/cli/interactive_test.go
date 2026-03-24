package cli

import "testing"

type interactiveTestStore struct{}

func (interactiveTestStore) Get(vault, key string) (string, error) { return "", nil }
func (interactiveTestStore) Set(vault, key, value string) error    { return nil }
func (interactiveTestStore) SetWithProtection(vault, key, value string, protected bool) error {
	return nil
}
func (interactiveTestStore) Delete(vault, key string) error      { return nil }
func (interactiveTestStore) List(vault string) ([]string, error) { return nil, nil }
func (interactiveTestStore) ListMetadata(vault string) ([]SecretMetadata, error) {
	return nil, nil
}
func (interactiveTestStore) ProtectAll(vault string) (int, error) { return 0, nil }

type interactiveTestBulk struct{ interactiveTestStore }

func (interactiveTestBulk) BulkSet(entries map[string]string, vault string) (int, error) {
	return len(entries), nil
}
func (interactiveTestBulk) BulkSetWithProtection(entries map[string]string, vault string, protected bool) (int, error) {
	return len(entries), nil
}
func (interactiveTestBulk) GetAll(vault string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (interactiveTestBulk) ReadRawService(service string) (map[string]string, error) {
	return map[string]string{}, nil
}

type interactiveTestVaults struct{}

func (interactiveTestVaults) List() ([]string, error)  { return []string{"default"}, nil }
func (interactiveTestVaults) Create(name string) error { return nil }
func (interactiveTestVaults) Active() (string, error)  { return "default", nil }
func (interactiveTestVaults) Switch(name string) error { return nil }

type interactiveTestClipboard struct{}

func (interactiveTestClipboard) Copy(value string) error { return nil }

func newInteractiveTestApp() *App {
	return &App{
		Store:     interactiveTestStore{},
		Bulk:      interactiveTestBulk{},
		Vaults:    interactiveTestVaults{},
		Clipboard: interactiveTestClipboard{},
	}
}

func TestRootNoArgsLaunchesInteractiveMode(t *testing.T) {
	app := newInteractiveTestApp()
	called := false
	previous := runInteractive
	runInteractive = func(_ interactiveDeps) error {
		called = true
		return nil
	}
	defer func() { runInteractive = previous }()

	root := NewRootCmd(app)
	root.SetArgs(nil)

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("interactive mode was not launched")
	}
}

func TestInteractiveFlagLaunchesInteractiveMode(t *testing.T) {
	app := newInteractiveTestApp()
	called := false
	previous := runInteractive
	runInteractive = func(deps interactiveDeps) error {
		called = true
		if deps.InitialFilter != "default" {
			t.Fatalf("InitialFilter = %q, want default from --vault", deps.InitialFilter)
		}
		return nil
	}
	defer func() { runInteractive = previous }()

	root := NewRootCmd(app)
	root.SetArgs([]string{"-i", "--vault", "default"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("interactive mode was not launched")
	}
}
