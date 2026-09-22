package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestTCPProxyWakeAndLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var backend *net.TCPListener
	wake := make(chan struct{}, 1)
	service := New(time.Hour, Hooks{
		Prepare: func(ports Ports) error {
			listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: ports.PaperInternal})
			if err != nil {
				return err
			}
			backend = listener
			go func() {
				for {
					connection, err := listener.AcceptTCP()
					if err != nil {
						return
					}
					go func() {
						defer connection.Close()
						_, _ = io.Copy(connection, connection)
					}()
				}
			}()
			return nil
		},
		Wake: func(context.Context) error {
			select {
			case wake <- struct{}{}:
			default:
			}
			return nil
		},
	})
	ports, err := service.Start(ctx, 35465, 35466)
	if err != nil {
		if backend != nil {
			_ = backend.Close()
		}
		t.Skipf("portas de teste indisponíveis: %v", err)
	}
	defer func() {
		_ = service.Close()
		if backend != nil {
			_ = backend.Close()
		}
	}()
	health := service.Health()
	if !health.TCPListening || !health.UDPListening {
		t.Fatalf("listeners deveriam estar ativos: %+v", health)
	}
	client, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", itoa(ports.JavaPublic)), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	if _, err := io.ReadFull(client, buffer); err != nil {
		t.Fatal(err)
	}
	if string(buffer) != "ping" {
		t.Fatalf("eco inesperado: %q", string(buffer))
	}
	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("callback Wake não foi chamado")
	}
}

func TestCloseReleasesListenersAndAllowsRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := New(time.Hour, Hooks{Prepare: func(Ports) error { return nil }})
	ports, err := service.Start(ctx, 37465, 37466)
	if err != nil {
		t.Skipf("portas de teste indisponíveis: %v", err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	if health := service.Health(); health.TCPListening || health.UDPListening {
		t.Fatalf("listeners permaneceram ativos após Close: %+v", health)
	}
	tcp, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4zero, Port: ports.JavaPublic})
	if err != nil {
		t.Fatalf("porta TCP não foi liberada: %v", err)
	}
	_ = tcp.Close()
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: ports.BedrockPublic})
	if err != nil {
		t.Fatalf("porta UDP não foi liberada: %v", err)
	}
	_ = udp.Close()
	if _, err := service.Start(ctx, ports.JavaPublic, ports.BedrockPublic); err != nil {
		t.Fatalf("reinício do proxy falhou: %v", err)
	}
	defer service.Close()
}

func itoa(value int) string {
	return fmt.Sprintf("%d", value)
}
