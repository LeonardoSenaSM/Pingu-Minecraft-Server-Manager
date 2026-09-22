package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const scanLimit = 1000

type Ports struct {
	JavaPublic     int
	BedrockPublic  int
	PaperInternal  int
	GeyserInternal int
	LocalIPs       []string
}

// Health is a point-in-time view of the actual listener sockets. Unlike a
// configured/desired state, TCPListening and UDPListening are only true while
// the respective OS socket descriptor can still be accessed.
type Health struct {
	TCPListening bool
	UDPListening bool
	Ports        Ports
}

type Hooks struct {
	Prepare func(Ports) error
	Wake    func(context.Context) error
	CanIdle func() bool
	OnIdle  func()
	Log     func(level, message string, err error)
}

type Service struct {
	mu          sync.RWMutex
	startMu     sync.Mutex
	spawnMu     sync.Mutex
	closing     bool
	active      bool
	ports       Ports
	tcpListener *net.TCPListener
	udpListener *net.UDPConn
	cancel      context.CancelFunc
	hooks       Hooks
	idleTimeout time.Duration
	lastUnix    atomic.Int64
	tcpClients  atomic.Int64
	udpClients  atomic.Int64
	wg          sync.WaitGroup
	udpMu       sync.Mutex
	udpSessions map[string]*udpSession
}

type udpSession struct {
	remote  *net.UDPAddr
	packets chan []byte
}

func New(idleTimeout time.Duration, hooks Hooks) *Service {
	service := &Service{idleTimeout: idleTimeout, hooks: hooks, udpSessions: make(map[string]*udpSession)}
	service.touch()
	return service
}

func (s *Service) Start(parent context.Context, javaStart, bedrockStart int) (Ports, error) {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if err := parent.Err(); err != nil {
		return Ports{}, err
	}
	s.mu.RLock()
	if s.active {
		ports := s.ports
		s.mu.RUnlock()
		return ports, nil
	}
	s.mu.RUnlock()

	tcpListener, javaPort, err := listenTCPFrom(javaStart)
	if err != nil {
		return Ports{}, fmt.Errorf("reservar porta Java TCP: %w", err)
	}
	udpListener, bedrockPort, err := listenUDPFrom(bedrockStart)
	if err != nil {
		_ = tcpListener.Close()
		return Ports{}, fmt.Errorf("reservar porta Bedrock UDP: %w", err)
	}
	ports := Ports{JavaPublic: javaPort, BedrockPublic: bedrockPort, LocalIPs: LocalIPv4Addresses()}
	ports.PaperInternal, err = findFreeTCPPort(javaPort + 1)
	if err == nil {
		ports.GeyserInternal, err = findFreeUDPPort(bedrockPort + 1)
	}
	if err != nil {
		_ = tcpListener.Close()
		_ = udpListener.Close()
		return Ports{}, err
	}
	if s.hooks.Prepare != nil {
		if err := s.hooks.Prepare(ports); err != nil {
			_ = tcpListener.Close()
			_ = udpListener.Close()
			return Ports{}, fmt.Errorf("preparar backends: %w", err)
		}
	}
	if err := parent.Err(); err != nil {
		_ = tcpListener.Close()
		_ = udpListener.Close()
		return Ports{}, err
	}
	ctx, cancel := context.WithCancel(parent)
	s.spawnMu.Lock()
	s.closing = false
	s.spawnMu.Unlock()
	s.mu.Lock()
	s.active = true
	s.ports = ports
	s.tcpListener = tcpListener
	s.udpListener = udpListener
	s.cancel = cancel
	s.mu.Unlock()
	s.touch()
	s.goTracked(func() { s.tcpAcceptLoop(ctx, tcpListener) })
	s.goTracked(func() { s.udpAcceptLoop(ctx, udpListener) })
	s.goTracked(func() { s.idleLoop(ctx) })
	s.log("info", fmt.Sprintf("listeners ativos: TCP %d e UDP %d", javaPort, bedrockPort), nil)
	return ports, nil
}

func (s *Service) Close() error {
	s.startMu.Lock()
	s.spawnMu.Lock()
	s.closing = true
	s.spawnMu.Unlock()
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		s.startMu.Unlock()
		return nil
	}
	cancel := s.cancel
	tcpListener := s.tcpListener
	udpListener := s.udpListener
	s.active = false
	s.cancel = nil
	s.tcpListener = nil
	s.udpListener = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	var errs []error
	if tcpListener != nil {
		if err := tcpListener.Close(); err != nil && !isClosed(err) {
			errs = append(errs, err)
		}
	}
	if udpListener != nil {
		if err := udpListener.Close(); err != nil && !isClosed(err) {
			errs = append(errs, err)
		}
	}
	s.wg.Wait()
	s.startMu.Unlock()
	return errors.Join(errs...)
}

func (s *Service) Status() (bool, Ports) {
	health := s.Health()
	return health.TCPListening && health.UDPListening, health.Ports
}

// Health validates the live descriptors without connecting to the public
// ports. A TCP dial would itself look like a player and incorrectly wake Paper.
func (s *Service) Health() Health {
	s.mu.RLock()
	active := s.active
	tcpListener := s.tcpListener
	udpListener := s.udpListener
	ports := s.ports
	ports.LocalIPs = append([]string(nil), ports.LocalIPs...)
	s.mu.RUnlock()
	if !active {
		return Health{Ports: ports}
	}
	return Health{
		TCPListening: tcpSocketOpen(tcpListener),
		UDPListening: udpSocketOpen(udpListener),
		Ports:        ports,
	}
}

func (s *Service) SessionCount() int {
	return int(s.tcpClients.Load() + s.udpClients.Load())
}

func (s *Service) goTracked(task func()) bool {
	s.spawnMu.Lock()
	defer s.spawnMu.Unlock()
	if s.closing {
		return false
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		task()
	}()
	return true
}

func (s *Service) tcpAcceptLoop(ctx context.Context, listener *net.TCPListener) {
	for {
		client, err := listener.AcceptTCP()
		if err != nil {
			if ctx.Err() != nil || isClosed(err) {
				return
			}
			s.log("error", "falha ao aceitar conexão TCP", err)
			continue
		}
		if !s.goTracked(func() { s.handleTCP(ctx, client) }) {
			_ = client.Close()
			return
		}
	}
}

func (s *Service) handleTCP(ctx context.Context, client *net.TCPConn) {
	defer client.Close()
	defer s.touch()
	s.tcpClients.Add(1)
	defer s.tcpClients.Add(-1)
	s.touch()
	s.log("info", "conexão Java recebida de "+client.RemoteAddr().String(), nil)
	if err := s.wake(ctx); err != nil {
		s.log("error", "não foi possível iniciar o Paper", err)
		return
	}
	paperAddress := s.paperAddress()
	if err := waitForTCP(ctx, paperAddress, 3*time.Minute); err != nil {
		s.log("error", "Paper não ficou pronto dentro do prazo", err)
		return
	}
	backend, err := net.DialTimeout("tcp4", paperAddress, 10*time.Second)
	if err != nil {
		s.log("error", "falha ao conectar ao Paper", err)
		return
	}
	defer backend.Close()
	trackedClient := &activityConn{Conn: client, touch: s.touch}
	trackedBackend := &activityConn{Conn: backend, touch: s.touch}
	done := make(chan error, 2)
	go copyConnection(done, trackedBackend, trackedClient)
	go copyConnection(done, trackedClient, trackedBackend)
	select {
	case <-ctx.Done():
	case err := <-done:
		if err != nil && !isClosed(err) {
			s.log("warning", "conexão Java encerrada com erro", err)
		}
	}
}

func copyConnection(done chan<- error, destination, source net.Conn) {
	_, err := io.Copy(destination, source)
	if tcp, ok := destination.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
	done <- err
}

func (s *Service) udpAcceptLoop(ctx context.Context, listener *net.UDPConn) {
	buffer := make([]byte, 64*1024)
	for {
		n, remote, err := listener.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil || isClosed(err) {
				return
			}
			s.log("error", "falha ao receber pacote Bedrock", err)
			continue
		}
		packet := append([]byte(nil), buffer[:n]...)
		key := remote.String()
		s.udpMu.Lock()
		session := s.udpSessions[key]
		if session == nil {
			session = &udpSession{remote: remote, packets: make(chan []byte, 128)}
			s.udpSessions[key] = session
			if !s.goTracked(func() { s.runUDPSession(ctx, listener, key, session) }) {
				delete(s.udpSessions, key)
				s.udpMu.Unlock()
				return
			}
		}
		s.udpMu.Unlock()
		s.touch()
		select {
		case session.packets <- packet:
		default:
			s.log("warning", "fila UDP cheia para "+key+"; pacote descartado", nil)
		}
	}
}

func (s *Service) runUDPSession(ctx context.Context, listener *net.UDPConn, key string, session *udpSession) {
	s.udpClients.Add(1)
	defer s.udpClients.Add(-1)
	defer func() {
		s.udpMu.Lock()
		if s.udpSessions[key] == session {
			delete(s.udpSessions, key)
		}
		s.udpMu.Unlock()
		s.touch()
	}()
	s.log("info", "tráfego Bedrock recebido de "+key, nil)
	if err := s.wake(ctx); err != nil {
		s.log("error", "não foi possível iniciar o Paper para Bedrock", err)
		return
	}
	if err := waitForTCP(ctx, s.paperAddress(), 3*time.Minute); err != nil {
		s.log("error", "Paper não ficou pronto para o Geyser", err)
		return
	}
	backendAddress, err := net.ResolveUDPAddr("udp4", s.geyserAddress())
	if err != nil {
		s.log("error", "endereço interno do Geyser inválido", err)
		return
	}
	backend, err := net.DialUDP("udp4", nil, backendAddress)
	if err != nil {
		s.log("error", "falha ao conectar ao Geyser", err)
		return
	}
	defer backend.Close()
	readerDone := make(chan error, 1)
	go func() {
		response := make([]byte, 64*1024)
		for {
			_ = backend.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, readErr := backend.Read(response)
			if readErr != nil {
				if timeout, ok := readErr.(net.Error); ok && timeout.Timeout() {
					if ctx.Err() != nil {
						readerDone <- ctx.Err()
						return
					}
					continue
				}
				readerDone <- readErr
				return
			}
			s.touch()
			if _, writeErr := listener.WriteToUDP(response[:n], session.remote); writeErr != nil {
				readerDone <- writeErr
				return
			}
		}
	}()
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-readerDone:
			if err != nil && !isClosed(err) && !errors.Is(err, context.Canceled) {
				s.log("warning", "sessão Bedrock encerrada com erro", err)
			}
			return
		case packet := <-session.packets:
			if _, err := backend.Write(packet); err != nil {
				s.log("warning", "falha ao encaminhar pacote ao Geyser", err)
				return
			}
			s.touch()
			resetTimer(timer, 2*time.Minute)
		case <-timer.C:
			return
		}
	}
}

func (s *Service) idleLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			inactive := time.Since(time.Unix(0, s.lastUnix.Load())) >= s.idleTimeout
			if inactive && s.SessionCount() == 0 && (s.hooks.CanIdle == nil || s.hooks.CanIdle()) && s.hooks.OnIdle != nil {
				s.hooks.OnIdle()
				s.touch()
			}
		}
	}
}

func (s *Service) wake(ctx context.Context) error {
	if s.hooks.Wake == nil {
		return errors.New("callback de inicialização não configurado")
	}
	return s.hooks.Wake(ctx)
}

func (s *Service) paperAddress() string {
	s.mu.RLock()
	port := s.ports.PaperInternal
	s.mu.RUnlock()
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func (s *Service) geyserAddress() string {
	s.mu.RLock()
	port := s.ports.GeyserInternal
	s.mu.RUnlock()
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func (s *Service) touch() { s.lastUnix.Store(time.Now().UnixNano()) }

func (s *Service) log(level, message string, err error) {
	if s.hooks.Log != nil {
		s.hooks.Log(level, message, err)
	}
}

type activityConn struct {
	net.Conn
	touch func()
}

func (c *activityConn) Read(buffer []byte) (int, error) {
	n, err := c.Conn.Read(buffer)
	if n > 0 {
		c.touch()
	}
	return n, err
}

func (c *activityConn) Write(buffer []byte) (int, error) {
	n, err := c.Conn.Write(buffer)
	if n > 0 {
		c.touch()
	}
	return n, err
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

func waitForTCP(ctx context.Context, address string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		connection, err := net.DialTimeout("tcp4", address, 800*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("tempo limite excedido")
		case <-ticker.C:
		}
	}
}

func listenTCPFrom(start int) (*net.TCPListener, int, error) {
	for port := start; port < start+scanLimit && port <= 65535; port++ {
		listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4zero, Port: port})
		if err == nil {
			return listener, port, nil
		}
	}
	return nil, 0, fmt.Errorf("nenhuma porta livre a partir de %d", start)
}

func listenUDPFrom(start int) (*net.UDPConn, int, error) {
	for port := start; port < start+scanLimit && port <= 65535; port++ {
		listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port})
		if err == nil {
			return listener, port, nil
		}
	}
	return nil, 0, fmt.Errorf("nenhuma porta livre a partir de %d", start)
}

func findFreeTCPPort(start int) (int, error) {
	listener, port, err := listenTCPFromLoopback(start)
	if listener != nil {
		_ = listener.Close()
	}
	return port, err
}

func listenTCPFromLoopback(start int) (*net.TCPListener, int, error) {
	for port := start; port < start+scanLimit && port <= 65535; port++ {
		listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err == nil {
			return listener, port, nil
		}
	}
	return nil, 0, errors.New("nenhuma porta TCP interna livre")
}

func findFreeUDPPort(start int) (int, error) {
	for port := start; port < start+scanLimit && port <= 65535; port++ {
		listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err == nil {
			_ = listener.Close()
			return port, nil
		}
	}
	return 0, errors.New("nenhuma porta UDP interna livre")
}

func LocalIPv4Addresses() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return []string{"127.0.0.1"}
	}
	private := make([]string, 0)
	other := make([]string, 0)
	seen := make(map[string]bool)
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := networkInterface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err != nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			text := ip.To4().String()
			if seen[text] {
				continue
			}
			seen[text] = true
			if ip.IsPrivate() {
				private = append(private, text)
			} else {
				other = append(other, text)
			}
		}
	}
	sort.Strings(private)
	sort.Strings(other)
	result := append(private, other...)
	if len(result) == 0 {
		return []string{"127.0.0.1"}
	}
	return result
}

func tcpSocketOpen(listener *net.TCPListener) bool {
	if listener == nil {
		return false
	}
	raw, err := listener.SyscallConn()
	if err != nil {
		return false
	}
	accessible := false
	err = raw.Control(func(uintptr) { accessible = true })
	return err == nil && accessible
}

func udpSocketOpen(listener *net.UDPConn) bool {
	if listener == nil {
		return false
	}
	raw, err := listener.SyscallConn()
	if err != nil {
		return false
	}
	accessible := false
	err = raw.Control(func(uintptr) { accessible = true })
	return err == nil && accessible
}

func isClosed(err error) bool {
	return errors.Is(err, net.ErrClosed) || strings.Contains(strings.ToLower(err.Error()), "use of closed network connection")
}
