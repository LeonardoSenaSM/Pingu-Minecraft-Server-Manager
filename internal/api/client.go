package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const LatestVersion = "__latest__"

type ProgressFunc func(downloaded, total int64)

type Client struct {
	http *http.Client
}

func NewClient(timeout time.Duration) *Client {
	return &Client{http: &http.Client{
		Timeout: timeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("limite de redirecionamentos HTTP excedido")
			}
			return nil
		},
	}}
}

func (c *Client) PaperVersions(ctx context.Context) ([]string, error) {
	var project struct {
		Versions map[string][]string `json:"versions"`
	}
	if err := c.getJSON(ctx, "https://fill.papermc.io/v3/projects/paper", &project); err != nil {
		return nil, fmt.Errorf("consultar versões do Paper: %w", err)
	}
	seen := make(map[string]bool)
	versions := make([]string, 0)
	for _, group := range project.Versions {
		for _, version := range group {
			if !seen[version] && !strings.Contains(version, "-") {
				seen[version] = true
				versions = append(versions, version)
			}
		}
	}
	if len(versions) == 0 {
		return nil, errors.New("a API do Paper não retornou versões estáveis")
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersions(versions[i], versions[j]) > 0 })
	return append([]string{LatestVersion}, versions...), nil
}

func (c *Client) ResolvePaper(ctx context.Context, selected string) (downloadURL, version string, err error) {
	versions, err := c.PaperVersions(ctx)
	if err != nil {
		return "", "", err
	}
	versions = versions[1:]
	if selected != "" && selected != LatestVersion {
		found := false
		for _, candidate := range versions {
			if candidate == selected {
				found = true
				break
			}
		}
		if !found {
			return "", "", fmt.Errorf("versão %s não encontrada no PaperMC", selected)
		}
		versions = []string{selected}
	}
	for _, candidate := range versions {
		var builds []struct {
			Channel   string `json:"channel"`
			Downloads map[string]struct {
				URL string `json:"url"`
			} `json:"downloads"`
		}
		endpoint := "https://fill.papermc.io/v3/projects/paper/versions/" + url.PathEscape(candidate) + "/builds"
		if getErr := c.getJSON(ctx, endpoint, &builds); getErr != nil {
			if selected != "" && selected != LatestVersion {
				return "", "", getErr
			}
			continue
		}
		for index := len(builds) - 1; index >= 0; index-- {
			build := builds[index]
			if build.Channel != "STABLE" {
				continue
			}
			if artifact, ok := build.Downloads["server:default"]; ok && artifact.URL != "" {
				return artifact.URL, candidate, nil
			}
		}
	}
	return "", "", errors.New("nenhum build estável do Paper foi encontrado")
}

func (c *Client) ModrinthDownload(ctx context.Context, slug, minecraftVersion string) (string, error) {
	query := url.Values{}
	query.Set("loaders", `["paper"]`)
	if minecraftVersion != "" {
		query.Set("game_versions", fmt.Sprintf(`["%s"]`, minecraftVersion))
	}
	endpoint := "https://api.modrinth.com/v2/project/" + url.PathEscape(slug) + "/version?" + query.Encode()
	var versions []struct {
		Files []struct {
			URL     string `json:"url"`
			Primary bool   `json:"primary"`
		} `json:"files"`
	}
	if err := c.getJSON(ctx, endpoint, &versions); err != nil {
		return "", err
	}
	if len(versions) == 0 || len(versions[0].Files) == 0 {
		if minecraftVersion != "" {
			return c.ModrinthDownload(ctx, slug, "")
		}
		return "", errors.New("nenhum arquivo Paper compatível foi encontrado")
	}
	for _, file := range versions[0].Files {
		if file.Primary {
			return file.URL, nil
		}
	}
	return versions[0].Files[0].URL, nil
}

func (c *Client) Download(ctx context.Context, downloadURL, destination string, progress ProgressFunc) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "MineServer/2.0")
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
	reader := &progressReader{reader: resp.Body, total: resp.ContentLength, notify: progress}
	written, err := io.Copy(tmp, reader)
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
	if err := replaceFile(tmpName, destination); err != nil {
		return err
	}
	committed = true
	if progress != nil {
		progress(written, written)
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "MineServer/2.0")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8*1024*1024)).Decode(target); err != nil {
		return fmt.Errorf("decodificar resposta JSON: %w", err)
	}
	return nil
}

type progressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	notify     ProgressFunc
	last       time.Time
}

func (r *progressReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	if n > 0 {
		r.downloaded += int64(n)
		if r.notify != nil && (time.Since(r.last) >= 100*time.Millisecond || err == io.EOF) {
			r.last = time.Now()
			r.notify(r.downloaded, r.total)
		}
	}
	return n, err
}

func replaceFile(source, destination string) error {
	if err := os.Rename(source, destination); err == nil {
		return nil
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(source, destination)
}

func compareVersions(a, b string) int {
	partsA := strings.SplitN(a, "-", 2)
	partsB := strings.SplitN(b, "-", 2)
	numbersA := strings.Split(partsA[0], ".")
	numbersB := strings.Split(partsB[0], ".")
	length := len(numbersA)
	if len(numbersB) > length {
		length = len(numbersB)
	}
	for index := 0; index < length; index++ {
		var numberA, numberB int
		if index < len(numbersA) {
			numberA, _ = strconv.Atoi(numbersA[index])
		}
		if index < len(numbersB) {
			numberB, _ = strconv.Atoi(numbersB[index])
		}
		if numberA != numberB {
			if numberA > numberB {
				return 1
			}
			return -1
		}
	}
	var suffixA, suffixB string
	if len(partsA) == 2 {
		suffixA = partsA[1]
	}
	if len(partsB) == 2 {
		suffixB = partsB[1]
	}
	if suffixA == "" && suffixB != "" {
		return 1
	}
	if suffixA != "" && suffixB == "" {
		return -1
	}
	return strings.Compare(suffixA, suffixB)
}
