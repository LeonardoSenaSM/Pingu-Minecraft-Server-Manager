package manager

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"mineserver/internal/api"
)

func testJar(t *testing.T, label string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("plugin.yml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("name: " + label + "\n"))
	if label == "geyser" {
		config, _ := writer.Create("config.yml")
		_, _ = config.Write([]byte("bedrock:\n  address: 0.0.0.0\n  port: 19132\n  server-name: \"x\"\nremote:\n  address: auto\n  port: 25565\n  auth-type: online\n"))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

type fakeRepos struct {
	server     *httptest.Server
	geyserJar  []byte
	brokenJar  atomic.Bool
	modrinthOK atomic.Bool
}

func newFakeRepos(t *testing.T, version string) *fakeRepos {
	t.Helper()
	repos := &fakeRepos{geyserJar: testJar(t, "geyser")}
	repos.modrinthOK.Store(true)
	paperJar := testJar(t, "paper")
	paperSum := sha256.Sum256(paperJar)
	floodgate := testJar(t, "floodgate")
	plugin := testJar(t, "plugin")
	mux := http.NewServeMux()
	mux.HandleFunc("/paper", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"versions":{"x":[%q]}}`, version)
	})
	mux.HandleFunc("/paper/versions/"+version+"/builds", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"id":7,"channel":"STABLE","downloads":{"server:default":{"url":%q,"checksums":{"sha256":%q}}}}]`,
			repos.server.URL+"/files/paper.jar", hex.EncodeToString(paperSum[:]))
	})
	mux.HandleFunc("/files/paper.jar", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(paperJar) })
	for _, project := range []string{"geyser", "floodgate"} {
		project := project
		mux.HandleFunc("/geyser/projects/"+project+"/versions/latest/builds/latest", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"version":"2.9.0","downloads":{"spigot":{}}}`)
		})
		mux.HandleFunc("/geyser/projects/"+project+"/versions/latest/builds/latest/downloads/spigot", func(w http.ResponseWriter, r *http.Request) {
			switch {
			case project == "floodgate" && repos.brokenJar.Load():
				_, _ = w.Write([]byte("corrompido"))
			case project == "floodgate":
				_, _ = w.Write(floodgate)
			default:
				_, _ = w.Write(repos.geyserJar)
			}
		})
	}
	mux.HandleFunc("/modrinth/project/", func(w http.ResponseWriter, r *http.Request) {
		if !repos.modrinthOK.Load() {
			fmt.Fprint(w, `[]`)
			return
		}
		fmt.Fprintf(w, `[{"version_number":"1.0","files":[{"url":%q,"primary":true}]}]`, repos.server.URL+"/files/plugin.jar")
	})
	mux.HandleFunc("/files/plugin.jar", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(plugin) })
	repos.server = httptest.NewServer(mux)
	t.Cleanup(repos.server.Close)
	return repos
}

func managerWithRepos(t *testing.T, repos *fakeRepos) *Manager {
	t.Helper()
	m := New(t.TempDir())
	t.Cleanup(func() { _ = m.Close() })
	m.api.SetEndpoints(api.Endpoints{
		Paper: repos.server.URL + "/paper", Geyser: repos.server.URL + "/geyser", Modrinth: repos.server.URL + "/modrinth",
	})
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestUpdateDependenciesInstallsFullSet(t *testing.T) {
	repos := newFakeRepos(t, "1.21.10")
	m := managerWithRepos(t, repos)
	if err := m.UpdateDependencies(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !m.dependenciesPresent() {
		t.Fatal("dependências obrigatórias ausentes após a instalação")
	}
	for _, name := range []string{"Chunky.jar", "CoreProtect.jar"} {
		if _, err := os.Stat(filepath.Join(m.pluginsDir, name)); err != nil {
			t.Fatalf("%s não instalado: %v", name, err)
		}
	}
	if got := m.InstalledPaperVersion(); got != "1.21.10" {
		t.Fatalf("versão instalada = %q", got)
	}
	if _, err := os.Stat(filepath.Join(m.serverDir, stagingDirName)); !os.IsNotExist(err) {
		t.Fatal("a pasta temporária deveria ser removida")
	}
}

func TestUpdateDependenciesFailureKeepsPreviousFiles(t *testing.T) {
	repos := newFakeRepos(t, "1.21.10")
	m := managerWithRepos(t, repos)
	if err := m.UpdateDependencies(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(m.serverDir, "server.jar"))
	if err != nil {
		t.Fatal(err)
	}
	repos.brokenJar.Store(true)
	if err := m.UpdateDependencies(context.Background()); err == nil {
		t.Fatal("download corrompido deveria falhar")
	}
	after, err := os.ReadFile(filepath.Join(m.serverDir, "server.jar"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("server.jar foi alterado mesmo com falha em outro componente")
	}
	if _, err := os.Stat(filepath.Join(m.pluginsDir, "Floodgate-Spigot.jar")); err != nil {
		t.Fatal("Floodgate anterior foi perdido")
	}
}

func TestOptionalPluginWithoutVersionIsKept(t *testing.T) {
	repos := newFakeRepos(t, "1.21.10")
	m := managerWithRepos(t, repos)
	chunky := filepath.Join(m.pluginsDir, "Chunky.jar")
	if err := os.WriteFile(chunky, []byte("versao-anterior"), 0o644); err != nil {
		t.Fatal(err)
	}
	repos.modrinthOK.Store(false)
	if err := m.UpdateDependencies(context.Background()); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(chunky); string(data) != "versao-anterior" {
		t.Fatal("plugin opcional sem versão compatível deveria ser mantido, não trocado")
	}
	if !strings.Contains(m.LogText(), "ainda não tem versão") {
		t.Fatal("o usuário deveria ser avisado no log")
	}
}

func TestVersionChangeBacksUpWorld(t *testing.T) {
	repos := newFakeRepos(t, "1.21.10")
	m := managerWithRepos(t, repos)
	if err := m.writeInstallation(PaperInstallation{Version: "1.21.9"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"world/level.dat", "world_nether/DIM-1/r.0.0.mca", "world/session.lock"} {
		full := filepath.Join(m.serverDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(path), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.UpdateDependencies(context.Background()); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(m.backupsDir(), "world-1.21.9-para-1.21.10-*.zip"))
	if len(matches) != 1 {
		t.Fatalf("esperado 1 backup do mundo, encontrados %v", matches)
	}
	archive, err := zip.OpenReader(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	names := map[string]bool{}
	for _, file := range archive.File {
		names[file.Name] = true
	}
	if !names["world/level.dat"] || !names["world_nether/DIM-1/r.0.0.mca"] || names["world/session.lock"] {
		t.Fatalf("conteúdo do backup inesperado: %v", names)
	}
}

func TestPruneBackupsKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"jars-20260101-000000", "jars-20260102-000000", "jars-20260103-000000", "jars-20260104-000000", "world-x.zip"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := pruneBackups(dir, "jars-", 2); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if strings.Join(got, ",") != "jars-20260103-000000,jars-20260104-000000,world-x.zip" {
		t.Fatalf("backups restantes: %v", got)
	}
}

func TestSetNestedYAMLValue(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"altera", "settings:\n  allow-end: true\n  connection-throttle: 4000\nticks-per:\n  animal-spawns: 400\n",
			"settings:\n  allow-end: true\n  connection-throttle: 0\nticks-per:\n  animal-spawns: 400\n"},
		{"adiciona na seção", "settings:\n  allow-end: true\nticks-per:\n  x: 1\n",
			"settings:\n  allow-end: true\n  connection-throttle: 0\nticks-per:\n  x: 1\n"},
		{"cria seção", "ticks-per:\n  x: 1\n", "ticks-per:\n  x: 1\nsettings:\n  connection-throttle: 0\n"},
		{"preserva CRLF", "settings:\r\n  connection-throttle: 4000\r\n", "settings:\r\n  connection-throttle: 0\r\n"},
	}
	for _, tc := range cases {
		got, changed := setNestedYAMLValue(tc.in, "settings", "connection-throttle", "0")
		if !changed || got != tc.want {
			t.Fatalf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
	if _, changed := setNestedYAMLValue("settings:\n  connection-throttle: 0\n", "settings", "connection-throttle", "0"); changed {
		t.Fatal("não deveria regravar quando o valor já está correto")
	}
}

func TestExistingEULAIsHonored(t *testing.T) {
	m := New(t.TempDir())
	if m.EULAAccepted() {
		t.Fatal("instalação nova não pode ter EULA aceita")
	}
	if err := os.MkdirAll(m.serverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.eulaPath(), []byte("# Gerado pelo Pingu\r\neula=true\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !m.EULAAccepted() {
		t.Fatal("eula.txt antigo com eula=true deveria continuar valendo")
	}
}

func TestInstallStagedRollsBackOnFailure(t *testing.T) {
	m := New(t.TempDir())
	staging := filepath.Join(m.serverDir, stagingDirName)
	if err := os.MkdirAll(filepath.Join(m.pluginsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(m.serverDir, "server.jar"), "paper-antigo")
	write(filepath.Join(m.pluginsDir, "Geyser-Spigot.jar"), "geyser-antigo")
	write(filepath.Join(staging, "server.jar"), "paper-novo")
	plan := []stagedFile{
		{dep: dependency{name: "PaperMC", relative: "server.jar"}, staged: filepath.Join(staging, "server.jar")},
		{dep: dependency{name: "Geyser", relative: filepath.Join("plugins", "Geyser-Spigot.jar")}, staged: filepath.Join(staging, "ausente.jar")},
	}
	err := m.installStaged(plan)
	if err == nil || !strings.Contains(err.Error(), "restaurados") {
		t.Fatalf("esperada falha com restauração, recebido %v", err)
	}
	for path, want := range map[string]string{
		filepath.Join(m.serverDir, "server.jar"):         "paper-antigo",
		filepath.Join(m.pluginsDir, "Geyser-Spigot.jar"): "geyser-antigo",
	} {
		if data, _ := os.ReadFile(path); string(data) != want {
			t.Fatalf("%s = %q, esperado %q", path, data, want)
		}
	}
}
