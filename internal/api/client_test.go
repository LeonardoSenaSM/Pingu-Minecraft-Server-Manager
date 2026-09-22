package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadIsAtomicOnCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		flusher, _ := writer.(http.Flusher)
		for index := 0; index < 100; index++ {
			select {
			case <-request.Context().Done():
				return
			default:
			}
			_, _ = writer.Write(make([]byte, 1024))
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(5 * time.Millisecond)
		}
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "server.jar")
	if err := os.WriteFile(destination, []byte("versao-anterior"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	client := NewClient(time.Minute)
	err := client.Download(ctx, server.URL, destination, func(downloaded, _ int64) {
		if downloaded >= 4096 {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("download cancelado deveria retornar erro")
	}
	data, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "versao-anterior" {
		t.Fatalf("arquivo anterior foi alterado: %q", data)
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("1.21.10", "1.21.9") <= 0 {
		t.Fatal("ordenação numérica incorreta")
	}
	if compareVersions("1.21", "1.21-rc1") <= 0 {
		t.Fatal("versão estável deve vir depois de pré-release")
	}
}
