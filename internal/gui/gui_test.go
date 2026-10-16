//go:build guitest

// Testes de fumaça da interface. Rode com: go test -tags guitest ./internal/gui
// O driver de teste do Fyne não é seguro para desenho concorrente e, em ~10%
// das execuções, trava dentro da biblioteca de fontes; por isso estes testes
// não fazem parte da suíte padrão.

package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mineserver/internal/api"
	"mineserver/internal/i18n"
	"mineserver/internal/manager"
)

// serialUI imita a thread principal do driver real: o driver de teste do
// Fyne executa fyne.Do na goroutine que chama, e a medição de texto não é
// segura para uso concorrente. Toda interação com widgets passa por aqui.
// Ao fim do teste a fila é fechada antes de o app de teste ser desmontado,
// para que goroutines atrasadas não desenhem num app já destruído.
type serialQueue struct {
	mu     sync.Mutex
	closed bool
}

func (q *serialQueue) run(f func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		f()
	}
}

func (q *serialQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
}

var serialUI *serialQueue

func buildTestUI(t *testing.T) (*UI, fyne.Window) {
	t.Helper()
	app := test.NewTempApp(t)
	app.Settings().SetTheme(theme.DarkTheme())
	queue := &serialQueue{}
	serialUI = queue
	t.Cleanup(queue.close)
	window := test.NewWindow(nil)
	service := manager.New(t.TempDir())
	t.Cleanup(func() { _ = service.Close() })
	// Nada de rede real nos testes: um servidor local responde às versões.
	repo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"versions":{"1.21":["1.21.10","1.21.9"]}}`))
	}))
	t.Cleanup(repo.Close)
	service.SetAPIEndpoints(api.Endpoints{Paper: repo.URL, Geyser: repo.URL, Modrinth: repo.URL})
	ui := New(app, window, service)
	ui.do = queue.run
	queue.run(func() { window.SetContent(ui.Build()) })
	window.Resize(fyne.NewSize(1180, 760))
	return ui, window
}

func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tempo esgotado: %s", what)
}

func TestBuildCreatesAllTabs(t *testing.T) {
	ui, _ := buildTestUI(t)
	if got := len(ui.tabs.Items); got != 5 {
		t.Fatalf("abas = %d, esperado 5", got)
	}
	if len(ui.translators) < 40 {
		t.Fatalf("poucos textos registrados para tradução: %d", len(ui.translators))
	}
	eventually(t, "lista de versões carregada", func() bool {
		var count int
		serialUI.run(func() { count = len(ui.controls.versions.Options) })
		return count == 3
	})
	eventually(t, "versão instalada carregada", func() bool {
		var loaded bool
		serialUI.run(func() { loaded = ui.installedLoaded })
		return loaded
	})
}

func TestLanguageSwitchRetranslatesEveryTab(t *testing.T) {
	ui, _ := buildTestUI(t)
	serialUI.run(func() {
		ui.settings.language.SetSelected(ui.loc.T("english"))
	})
	if ui.loc.Language() != i18n.English {
		t.Fatalf("idioma = %s", ui.loc.Language())
	}
	want := map[int]string{0: "Home & Status", 2: "Player Manager", 4: "Settings & Files"}
	for index, text := range want {
		if got := ui.tabs.Items[index].Text; got != text {
			t.Fatalf("aba %d = %q, esperado %q", index, got, text)
		}
	}
	if !strings.Contains(ui.controls.installed.Text, "Installed version") {
		t.Fatalf("rótulo da versão não traduzido: %q", ui.controls.installed.Text)
	}
	serialUI.run(func() {
		ui.settings.language.SetSelected(ui.loc.T("portuguese"))
	})
	if got := ui.tabs.Items[0].Text; got != "Início & Status" {
		t.Fatalf("aba 0 após voltar para PT = %q", got)
	}
}

func TestConsoleShowsBoundedTail(t *testing.T) {
	ui, _ := buildTestUI(t)
	for index := 0; index < consoleLines+300; index++ {
		ui.manager.Log("info", "teste", "linha", nil)
	}
	ui.manager.Log("info", "teste", "ultima-linha", nil)
	eventually(t, "console atualizado", func() bool {
		var text string
		serialUI.run(func() { text = ui.console.output.Text })
		return strings.Contains(text, "ultima-linha")
	})
	var lines int
	serialUI.run(func() { lines = strings.Count(ui.console.output.Text, "\n") + 1 })
	if lines > consoleLines {
		t.Fatalf("console exibe %d linhas, limite %d", lines, consoleLines)
	}
}

// O driver de teste do Fyne executa fyne.Do na própria goroutine que chama;
// por isso o diálogo é aberto aqui de forma síncrona. Que o PLAY devolve
// ErrEULANotAccepted antes de baixar qualquer coisa é coberto em
// manager.TestStartRequiresEULA.
func TestEULADialogIsShown(t *testing.T) {
	ui, window := buildTestUI(t)
	retried := false
	handled := ui.requireEULA(manager.ErrEULANotAccepted, func() { retried = true })
	if !handled {
		t.Fatal("requireEULA deveria tratar ErrEULANotAccepted")
	}
	found := false
	serialUI.run(func() {
		for _, overlay := range window.Canvas().Overlays().List() {
			if containsText(overlay, manager.EULAURL) {
				found = true
			}
		}
	})
	if !found {
		t.Fatal("diálogo da EULA com o link oficial não foi exibido")
	}
	if retried {
		t.Fatal("a operação não pode ser repetida sem o aceite")
	}
	if ui.requireEULA(os.ErrNotExist, func() {}) {
		t.Fatal("outros erros não devem abrir o diálogo da EULA")
	}
	if _, err := os.Stat(filepath.Join(ui.manager.ServerDir(), "eula.txt")); !os.IsNotExist(err) {
		t.Fatal("eula.txt não pode ser criado sem o aceite")
	}
}

func containsText(object fyne.CanvasObject, needle string) bool {
	switch typed := object.(type) {
	case *widget.Label:
		return strings.Contains(typed.Text, needle)
	case *widget.Hyperlink:
		return strings.Contains(typed.Text, needle)
	case fyne.Widget:
		renderer := test.WidgetRenderer(typed)
		for _, child := range renderer.Objects() {
			if containsText(child, needle) {
				return true
			}
		}
	case *fyne.Container:
		for _, child := range typed.Objects {
			if containsText(child, needle) {
				return true
			}
		}
	}
	return false
}
