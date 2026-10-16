package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func jarBytes(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("plugin.yml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("name: teste\n"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestLatestStableBuildIgnoresResponseOrder(t *testing.T) {
	build := func(id int, channel string) paperBuild {
		b := paperBuild{ID: id, Channel: channel}
		b.Downloads = map[string]struct {
			Name      string `json:"name"`
			URL       string `json:"url"`
			Checksums struct {
				SHA256 string `json:"sha256"`
			} `json:"checksums"`
		}{"server:default": {URL: fmt.Sprintf("https://example/%d.jar", id)}}
		return b
	}
	newestFirst := []paperBuild{build(48, "ALPHA"), build(47, "STABLE"), build(30, "STABLE")}
	oldestFirst := []paperBuild{build(30, "STABLE"), build(47, "STABLE"), build(48, "ALPHA")}
	for name, list := range map[string][]paperBuild{"newest-first": newestFirst, "oldest-first": oldestFirst} {
		got, ok := latestStableBuild(list)
		if !ok || got.ID != 47 {
			t.Fatalf("%s: build escolhido = %d, esperado 47", name, got.ID)
		}
	}
}

func TestResolvePaperPicksHighestStableBuild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.UserAgent(), "Pingu-Minecraft-Server-Manager/") {
			http.Error(w, "user-agent", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/paper":
			fmt.Fprint(w, `{"versions":{"1.21":["1.21.10","1.21.9","1.21.10-rc1"]}}`)
		case "/paper/versions/1.21.10/builds":
			fmt.Fprint(w, `[{"id":60,"channel":"STABLE","downloads":{"server:default":{"url":"https://x/60.jar","checksums":{"sha256":"AB"}}}},
			{"id":41,"channel":"STABLE","downloads":{"server:default":{"url":"https://x/41.jar"}}}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(time.Minute)
	client.paperAPI = server.URL + "/paper"
	artifact, err := client.ResolvePaper(context.Background(), LatestVersion, false)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.URL != "https://x/60.jar" || artifact.Version != "1.21.10" || artifact.SHA256 != "ab" {
		t.Fatalf("artefato inesperado: %+v", artifact)
	}
}

func TestModrinthNeverFallsBackToOtherVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "game_versions") {
			fmt.Fprint(w, `[]`)
			return
		}
		fmt.Fprint(w, `[{"version_number":"old","files":[{"url":"https://x/old.jar","primary":true}]}]`)
	}))
	defer server.Close()
	client := NewClient(time.Minute)
	client.modrinthAPI = server.URL
	_, err := client.ModrinthArtifact(context.Background(), "chunky", "99.1")
	if !errors.Is(err, ErrNoCompatibleVersion) {
		t.Fatalf("esperado ErrNoCompatibleVersion, recebido %v", err)
	}
}

func TestDownloadRejectsChecksumMismatchAndKeepsPrevious(t *testing.T) {
	content := jarBytes(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(content) }))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "server.jar")
	if err := os.WriteFile(destination, []byte("anterior"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewClient(time.Minute)
	err := client.Download(context.Background(), Artifact{Name: "Paper", URL: server.URL, SHA256: strings.Repeat("0", 64)}, destination, nil)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("esperado erro de checksum, recebido %v", err)
	}
	if data, _ := os.ReadFile(destination); string(data) != "anterior" {
		t.Fatal("arquivo anterior foi substituído apesar do checksum inválido")
	}
	sum := sha256.Sum256(content)
	if err := client.Download(context.Background(), Artifact{Name: "Paper", URL: server.URL, SHA256: hex.EncodeToString(sum[:])}, destination, nil); err != nil {
		t.Fatalf("download com checksum correto falhou: %v", err)
	}
}

func TestDownloadRejectsCorruptJar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>erro</html>")) }))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "Geyser-Spigot.jar")
	err := NewClient(time.Minute).Download(context.Background(), Artifact{Name: "Geyser", URL: server.URL}, destination, nil)
	if err == nil || !strings.Contains(err.Error(), "JAR") {
		t.Fatalf("esperado erro de JAR inválido, recebido %v", err)
	}
	if _, statErr := os.Stat(destination); !os.IsNotExist(statErr) {
		t.Fatal("JAR inválido não deveria ser gravado")
	}
}

func newChannelServer(t *testing.T) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/paper":
			fmt.Fprint(w, `{"versions":{"26":["26.3","26.2"],"1.21":["1.21.10"]}}`)
		case "/paper/versions/26.3/builds":
			fmt.Fprint(w, `[{"id":190,"channel":"BETA","downloads":{"server:default":{"url":"https://x/263-190.jar"}}},
			{"id":120,"channel":"ALPHA","downloads":{"server:default":{"url":"https://x/263-120.jar"}}}]`)
		case "/paper/versions/26.2/builds":
			fmt.Fprint(w, `[{"id":124,"channel":"STABLE","downloads":{"server:default":{"url":"https://x/262-124.jar"}}}]`)
		case "/paper/versions/1.21.10/builds":
			fmt.Fprint(w, `[{"id":130,"channel":"STABLE","downloads":{"server:default":{"url":"https://x/12110.jar"}}}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := NewClient(time.Minute)
	client.paperAPI = server.URL + "/paper"
	return client
}

func TestPaperVersionsOrderYearBasedVersionsFirst(t *testing.T) {
	versions, err := newChannelServer(t).PaperVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(versions[1:], ","); got != "26.3,26.2,1.21.10" {
		t.Fatalf("ordem das versões = %s", got)
	}
}

func TestLatestStableSkipsVersionWithOnlyBeta(t *testing.T) {
	artifact, err := newChannelServer(t).ResolvePaper(context.Background(), LatestVersion, true)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Version != "26.2" || artifact.Channel != ChannelStable {
		t.Fatalf("\"mais recente estável\" escolheu %s %s", artifact.Version, artifact.Channel)
	}
}

func TestExplicitBetaVersionNeedsConsent(t *testing.T) {
	client := newChannelServer(t)
	_, err := client.ResolvePaper(context.Background(), "26.3", false)
	var experimental *ExperimentalOnlyError
	if !errors.As(err, &experimental) || experimental.Version != "26.3" || experimental.Build != 190 {
		t.Fatalf("esperado ExperimentalOnlyError para 26.3, recebido %v", err)
	}
	artifact, err := client.ResolvePaper(context.Background(), "26.3", true)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Channel != ChannelBeta || artifact.Build != 190 {
		t.Fatalf("com consentimento deveria usar o BETA 190, recebido %+v", artifact)
	}
}
