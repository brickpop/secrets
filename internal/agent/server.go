package agent

import (
	"crypto/subtle"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/brickpop/secrets/internal/crypto"
	"github.com/brickpop/secrets/internal/store"
)

// Server holds decrypted store data in memory and serves it over a Unix socket.
type Server struct {
	data       map[string]string
	dataMu     sync.RWMutex
	passphrase string
	backend    crypto.Backend
	newBackend func(passphrase string) crypto.Backend
	storeDir   string
	sockPath   string
	done       chan struct{}
	ready      chan struct{}
}

// NewServer creates a new agent server.
// newBackend is a factory used by passwd to create a replacement backend with a new passphrase.
func NewServer(data map[string]string, sockPath string, passphrase string, backend crypto.Backend, newBackend func(string) crypto.Backend, storeDir string) *Server {
	return &Server{
		data:       data,
		sockPath:   sockPath,
		passphrase: passphrase,
		backend:    backend,
		newBackend: newBackend,
		storeDir:   storeDir,
		done:       make(chan struct{}),
		ready:      make(chan struct{}),
	}
}

// Start listens on the Unix socket and serves requests.
// ttl of 0 means no expiry. Blocks until Stop or TTL expiry.
func (s *Server) Start(ttl time.Duration) error {
	os.Remove(s.sockPath)

	ln, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return err
	}

	close(s.ready)

	os.Chmod(s.sockPath, 0600)

	if ttl > 0 {
		time.AfterFunc(ttl, func() {
			s.Stop()
		})
	}

	// Close listener when done fires (from Stop, TTL, or signal).
	go func() {
		<-s.done
		ln.Close()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		select {
		case <-sigCh:
			s.Stop()
		case <-s.done:
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
				continue
			}
		}
		go s.handleConn(conn)
	}
}

// Ready returns a channel that is closed when the server is listening.
func (s *Server) Ready() <-chan struct{} {
	return s.ready
}

// Stop wipes memory, closes the socket, and cleans up.
func (s *Server) Stop() {
	select {
	case <-s.done:
		return
	default:
	}
	close(s.done)

	s.dataMu.Lock()
	for k := range s.data {
		delete(s.data, k)
	}
	s.dataMu.Unlock()

	os.Remove(s.sockPath)
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	for {
		var req Request
		if err := ReadMsg(conn, &req); err != nil {
			return
		}

		resp := s.handleRequest(&req)
		// Write operations can take ~500ms (scrypt); use a generous deadline.
		conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if err := WriteMsg(conn, resp); err != nil {
			return
		}

		if _, ok := req.Payload.(*Request_Stop); ok {
			s.Stop()
			return
		}
	}
}

func (s *Server) handleRequest(req *Request) *Response {
	switch p := req.Payload.(type) {
	case *Request_Get:
		return s.handleGet(p.Get)
	case *Request_List:
		return s.handleList()
	case *Request_Set:
		return s.handleSet(p.Set)
	case *Request_Delete:
		return s.handleDelete(p.Delete)
	case *Request_Passwd:
		return s.handlePasswd(p.Passwd)
	case *Request_Stop:
		return &Response{Ok: true}
	default:
		return &Response{Ok: false, Error: "unknown op"}
	}
}

func (s *Server) handleGet(req *GetRequest) *Response {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()
	val, ok := s.data[req.Key]
	if !ok {
		return &Response{Ok: false, Error: "key not found"}
	}
	return &Response{Ok: true, Value: val}
}

func (s *Server) handleList() *Response {
	s.dataMu.RLock()
	defer s.dataMu.RUnlock()
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return &Response{Ok: true, Keys: keys}
}

func (s *Server) handleSet(req *SetRequest) *Response {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	// Overwriting an existing key requires the passphrase.
	if _, exists := s.data[req.Key]; exists {
		if !s.checkPassphrase(req.Passphrase) {
			return &Response{Ok: false, Error: ErrPassphraseRequired}
		}
	}

	s.data[req.Key] = req.Value

	if err := store.SaveData(s.data, s.backend, s.storeDir); err != nil {
		delete(s.data, req.Key)
		return &Response{Ok: false, Error: fmt.Sprintf("saving store: %v", err)}
	}

	return &Response{Ok: true}
}

func (s *Server) handleDelete(req *DeleteRequest) *Response {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	if !s.checkPassphrase(req.Passphrase) {
		return &Response{Ok: false, Error: ErrPassphraseRequired}
	}

	if _, exists := s.data[req.Key]; !exists {
		return &Response{Ok: false, Error: "key not found"}
	}

	delete(s.data, req.Key)

	if err := store.SaveData(s.data, s.backend, s.storeDir); err != nil {
		return &Response{Ok: false, Error: fmt.Sprintf("saving store: %v", err)}
	}

	return &Response{Ok: true}
}

func (s *Server) handlePasswd(req *PasswdRequest) *Response {
	s.dataMu.Lock()
	defer s.dataMu.Unlock()

	if !s.checkPassphrase(req.Passphrase) {
		return &Response{Ok: false, Error: ErrPassphraseRequired}
	}

	newBackend := s.newBackend(req.NewPassphrase)

	if err := store.SaveData(s.data, newBackend, s.storeDir); err != nil {
		return &Response{Ok: false, Error: fmt.Sprintf("saving store: %v", err)}
	}

	s.passphrase = req.NewPassphrase
	s.backend = newBackend

	return &Response{Ok: true}
}

func (s *Server) checkPassphrase(provided string) bool {
	return subtle.ConstantTimeCompare([]byte(s.passphrase), []byte(provided)) == 1
}
