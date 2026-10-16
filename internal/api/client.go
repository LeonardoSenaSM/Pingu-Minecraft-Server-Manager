package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const LatestVersion = "__latest__"

type ProgressFunc func(downloaded, total int64)

type Client struct {
	http        *http.Client
	paperAPI    string
	geyserAPI   string
	modrinthAPI string
}

// Endpoints permite apontar o cliente para servidores de teste.
type Endpoints struct {
	Paper, Geyser, Modrinth string
}

func (c *Client) SetEndpoints(endpoints Endpoints) {
	if endpoints.Paper != "" {
		c.paperAPI = endpoints.Paper
	}
	if endpoints.Geyser != "" {
		c.geyserAPI = endpoints.Geyser
	}
	if endpoints.Modrinth != "" {
		c.modrinthAPI = endpoints.Modrinth
	}
}

func NewClient(timeout time.Duration) *Client {
	return &Client{
		paperAPI:    "https://fill.papermc.io/v3/projects/paper",
		geyserAPI:   "https://download.geysermc.org/v2",
		modrinthAPI: "https://api.modrinth.com/v2",
		http: &http.Client{
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
	if err := c.getJSON(ctx, c.paperAPI, &project); err != nil {
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

func (c *Client) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent())
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
