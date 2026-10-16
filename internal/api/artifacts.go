package api

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

const projectURL = "https://github.com/LeonardoSenaSM/Pingu-Minecraft-Server-Manager"

// ErrNoCompatibleVersion indica que o projeto não publicou um arquivo para a
// versão de Minecraft pedida. Nunca há fallback silencioso para outra versão.
var ErrNoCompatibleVersion = errors.New("nenhuma versão compatível publicada")

var appVersion atomic.Value

// SetAppVersion define a versão usada no User-Agent. A API do Paper exige um
// User-Agent que identifique o software e traga um contato.
func SetAppVersion(version string) {
	if strings.TrimSpace(version) == "" {
		version = "development"
	}
	appVersion.Store(version)
}

func UserAgent() string {
	version, _ := appVersion.Load().(string)
	if version == "" {
		version = "development"
	}
	return "Pingu-Minecraft-Server-Manager/" + version + " (+" + projectURL + ")"
}

// Artifact descreve um arquivo a baixar e, quando a fonte publica, o hash
// esperado. O download falha se o hash não conferir.
type Artifact struct {
	Name    string
	URL     string
	Version string
	SHA256  string
	SHA512  string
	// Channel e Build só são preenchidos para o PaperMC.
	Channel string
	Build   int
}

type paperBuild struct {
	ID        int    `json:"id"`
	Channel   string `json:"channel"`
	Downloads map[string]struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		Checksums struct {
			SHA256 string `json:"sha256"`
		} `json:"checksums"`
	} `json:"downloads"`
}

// Canais de build do PaperMC, do mais confiável para o menos.
const (
	ChannelStable = "STABLE"
	ChannelBeta   = "BETA"
	ChannelAlpha  = "ALPHA"
)

// ExperimentalOnlyError indica que a versão escolhida ainda não tem build
// STABLE no PaperMC, apenas builds de teste (BETA). Acontece nas primeiras
// semanas após o lançamento de uma versão nova do Minecraft.
type ExperimentalOnlyError struct {
	Version string
	Channel string
	Build   int
}

func (e *ExperimentalOnlyError) Error() string {
	return fmt.Sprintf("o PaperMC ainda não publicou build estável para o Minecraft %s; o mais recente é o build %d (%s)", e.Version, e.Build, strings.ToLower(e.Channel))
}

// newestBuild escolhe o build de maior ID no canal pedido, sem depender da
// ordem em que a API devolve a lista.
func newestBuild(builds []paperBuild, channel string) (paperBuild, bool) {
	best, found := paperBuild{}, false
	for _, build := range builds {
		if !strings.EqualFold(build.Channel, channel) {
			continue
		}
		if download, ok := build.Downloads["server:default"]; !ok || download.URL == "" {
			continue
		}
		if !found || build.ID > best.ID {
			best, found = build, true
		}
	}
	return best, found
}

func latestStableBuild(builds []paperBuild) (paperBuild, bool) {
	return newestBuild(builds, ChannelStable)
}

func paperArtifact(version string, build paperBuild) Artifact {
	download := build.Downloads["server:default"]
	return Artifact{
		Name: "PaperMC", URL: download.URL, Version: version,
		SHA256:  strings.ToLower(download.Checksums.SHA256),
		Channel: strings.ToUpper(build.Channel), Build: build.ID,
	}
}

// ResolvePaper encontra o build mais recente da versão escolhida, sempre
// preferindo STABLE.
//
//   - Com LatestVersion ("Mais recente estável"), usa a versão de Minecraft
//     mais nova que tenha build STABLE; nunca escolhe build de teste.
//   - Com uma versão específica sem build STABLE, usa o BETA mais recente
//     apenas se allowExperimental for true; senão devolve
//     *ExperimentalOnlyError para a interface pedir confirmação.
//
// Builds ALPHA nunca são usados.
func (c *Client) ResolvePaper(ctx context.Context, selected string, allowExperimental bool) (Artifact, error) {
	versions, err := c.PaperVersions(ctx)
	if err != nil {
		return Artifact{}, err
	}
	versions = versions[1:]
	explicit := selected != "" && selected != LatestVersion
	if explicit {
		found := false
		for _, candidate := range versions {
			if candidate == selected {
				found = true
				break
			}
		}
		if !found {
			return Artifact{}, fmt.Errorf("versão %s não encontrada no PaperMC", selected)
		}
		versions = []string{selected}
	}
	var lastErr error
	for _, candidate := range versions {
		var builds []paperBuild
		endpoint := c.paperAPI + "/versions/" + url.PathEscape(candidate) + "/builds"
		if err := c.getJSON(ctx, endpoint, &builds); err != nil {
			if ctx.Err() != nil {
				return Artifact{}, ctx.Err()
			}
			lastErr = err
			continue
		}
		if build, ok := newestBuild(builds, ChannelStable); ok {
			return paperArtifact(candidate, build), nil
		}
		if !explicit {
			continue
		}
		beta, ok := newestBuild(builds, ChannelBeta)
		if !ok {
			if alpha, hasAlpha := newestBuild(builds, ChannelAlpha); hasAlpha {
				return Artifact{}, fmt.Errorf("o Minecraft %s ainda está em fase ALPHA no PaperMC (build %d); aguarde um build BETA ou STABLE", candidate, alpha.ID)
			}
			return Artifact{}, fmt.Errorf("o PaperMC não tem builds para o Minecraft %s", candidate)
		}
		if !allowExperimental {
			return Artifact{}, &ExperimentalOnlyError{Version: candidate, Channel: ChannelBeta, Build: beta.ID}
		}
		return paperArtifact(candidate, beta), nil
	}
	if lastErr != nil {
		return Artifact{}, fmt.Errorf("nenhum build estável do Paper foi encontrado: %w", lastErr)
	}
	return Artifact{}, errors.New("nenhum build estável do Paper foi encontrado")
}

// GeyserArtifact resolve o build mais recente do Geyser ou Floodgate para
// Spigot/Paper. O hash é usado quando a API o informa.
func (c *Client) GeyserArtifact(ctx context.Context, project string) (Artifact, error) {
	base := c.geyserAPI + "/projects/" + url.PathEscape(project) + "/versions/latest/builds/latest"
	artifact := Artifact{Name: project, URL: base + "/downloads/spigot"}
	var build struct {
		Version   string `json:"version"`
		Downloads map[string]struct {
			SHA256 string `json:"sha256"`
		} `json:"downloads"`
	}
	if err := c.getJSON(ctx, base, &build); err != nil {
		if ctx.Err() != nil {
			return Artifact{}, ctx.Err()
		}
		return Artifact{}, fmt.Errorf("consultar %s: %w", project, err)
	}
	artifact.Version = build.Version
	if spigot, ok := build.Downloads["spigot"]; ok {
		artifact.SHA256 = strings.ToLower(spigot.SHA256)
	}
	return artifact, nil
}

// ModrinthArtifact retorna o arquivo de plugin publicado para Paper na versão
// de Minecraft informada. Se não existir, retorna ErrNoCompatibleVersion.
func (c *Client) ModrinthArtifact(ctx context.Context, slug, minecraftVersion string) (Artifact, error) {
	if strings.TrimSpace(minecraftVersion) == "" {
		return Artifact{}, errors.New("versão de Minecraft não informada")
	}
	query := url.Values{}
	query.Set("loaders", `["paper"]`)
	query.Set("game_versions", fmt.Sprintf(`[%q]`, minecraftVersion))
	endpoint := c.modrinthAPI + "/project/" + url.PathEscape(slug) + "/version?" + query.Encode()
	var versions []struct {
		VersionNumber string `json:"version_number"`
		Files         []struct {
			URL     string            `json:"url"`
			Primary bool              `json:"primary"`
			Hashes  map[string]string `json:"hashes"`
		} `json:"files"`
	}
	if err := c.getJSON(ctx, endpoint, &versions); err != nil {
		return Artifact{}, err
	}
	for _, version := range versions {
		if len(version.Files) == 0 {
			continue
		}
		file := version.Files[0]
		for _, candidate := range version.Files {
			if candidate.Primary {
				file = candidate
				break
			}
		}
		return Artifact{
			Name: slug, URL: file.URL, Version: version.VersionNumber,
			SHA512: strings.ToLower(file.Hashes["sha512"]),
		}, nil
	}
	return Artifact{}, fmt.Errorf("%s para Minecraft %s: %w", slug, minecraftVersion, ErrNoCompatibleVersion)
}

// Download grava o artefato em destination de forma atômica. Confere o hash
// publicado e, para .jar, valida que o arquivo é um ZIP íntegro antes de
// substituir o arquivo existente.
func (c *Client) Download(ctx context.Context, artifact Artifact, destination string, progress ProgressFunc) error {
	if artifact.URL == "" {
		return errors.New("endereço de download vazio")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent())
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download retornou HTTP %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".download-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	var digest hash.Hash
	expected := ""
	switch {
	case artifact.SHA256 != "":
		digest, expected = sha256.New(), artifact.SHA256
	case artifact.SHA512 != "":
		digest, expected = sha512.New(), artifact.SHA512
	}
	writer := io.Writer(tmp)
	if digest != nil {
		writer = io.MultiWriter(tmp, digest)
	}
	reader := &progressReader{reader: resp.Body, total: resp.ContentLength, notify: progress}
	written, err := io.Copy(writer, reader)
	if err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if written == 0 {
		return errors.New("o arquivo recebido está vazio")
	}
	if digest != nil {
		if got := hex.EncodeToString(digest.Sum(nil)); got != expected {
			return fmt.Errorf("checksum de %s não confere (esperado %s, recebido %s)", artifact.Name, short(expected), short(got))
		}
	}
	if strings.EqualFold(filepath.Ext(destination), ".jar") {
		if err := ValidateJar(tmpName); err != nil {
			return fmt.Errorf("%s: %w", artifact.Name, err)
		}
	}
	if err := replaceFile(tmpName, destination); err != nil {
		return err
	}
	committed = true
	if progress != nil {
		progress(written, written)
	}
	return nil
}

// ValidateJar confirma que o arquivo é um ZIP legível com ao menos uma entrada.
func ValidateJar(path string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("arquivo JAR inválido: %w", err)
	}
	defer reader.Close()
	if len(reader.File) == 0 {
		return errors.New("arquivo JAR vazio")
	}
	return nil
}

func short(value string) string {
	if len(value) > 12 {
		return value[:12] + "…"
	}
	return value
}
