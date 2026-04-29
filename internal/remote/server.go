package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"vulpineos/internal/juggler"
)

const maxWebSocketMessageBytes int64 = 2 << 20

// Server exposes the VulpineOS kernel over WebSocket for remote TUI clients.
type Server struct {
	auth      *Authenticator
	client    *juggler.Client
	addr      string
	server    *http.Server
	mux       *http.ServeMux
	clients   map[*wsClient]struct{}
	clientsMu sync.RWMutex
	panelAPI  *PanelAPI

	writeTimeout time.Duration
}

type wsClient struct {
	conn    *websocket.Conn
	ctx     context.Context
	writeMu sync.Mutex
}

func (c *wsClient) write(ctx context.Context, typ websocket.MessageType, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("websocket connection unavailable")
	}
	return c.conn.Write(ctx, typ, data)
}

func (c *wsClient) close(status websocket.StatusCode, reason string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.Close(status, reason)
}

// NewServer creates a remote access server.
func NewServer(addr string, apiKey string, jugglerClient *juggler.Client) *Server {
	s := &Server{
		auth:    NewAuthenticator(apiKey),
		client:  jugglerClient,
		addr:    addr,
		clients: make(map[*wsClient]struct{}),

		writeTimeout: 2 * time.Second,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		setJSONSecurityHeaders(w)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/auth/check", func(w http.ResponseWriter, r *http.Request) {
		setJSONSecurityHeaders(w)
		if !s.auth.Validate(r) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	s.mux = mux
	s.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return s
}

func setJSONSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// SetPanelAPI attaches the PanelAPI for handling control messages from the web panel.
func (s *Server) SetPanelAPI(api *PanelAPI) {
	s.panelAPI = api
}

// Mux returns the HTTP mux for registering additional handlers (e.g., web panel).
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

// Start begins listening for WebSocket connections.
func (s *Server) Start() error {
	log.Printf("VulpineOS remote server listening on %s", s.addr)
	return s.server.ListenAndServe()
}

// StartTLS begins listening with TLS.
func (s *Server) StartTLS(certFile, keyFile string) error {
	log.Printf("VulpineOS remote server listening on %s (TLS)", s.addr)
	return s.server.ListenAndServeTLS(certFile, keyFile)
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	clients := s.snapshotClients()
	for _, c := range clients {
		_ = c.close(websocket.StatusGoingAway, "server shutting down")
	}
	s.removeClients(clients)
	return s.server.Shutdown(ctx)
}

// BroadcastEvent sends a Juggler event to all connected clients.
// Clients that fail to receive the message are removed.
func (s *Server) BroadcastEvent(method, sessionID string, params json.RawMessage) {
	msg, _ := json.Marshal(&juggler.Message{
		Method:    method,
		Params:    params,
		SessionID: sessionID,
	})
	env, err := NewJugglerEnvelope(msg)
	if err != nil {
		return
	}

	clients := s.snapshotClients()
	dead := make([]*wsClient, 0)
	for _, c := range clients {
		baseCtx := c.ctx
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		ctx, cancel := context.WithTimeout(baseCtx, s.writeTimeout)
		err := c.write(ctx, websocket.MessageText, env)
		cancel()
		if err != nil {
			dead = append(dead, c)
		}
	}

	s.removeClients(dead)
}

func (s *Server) snapshotClients() []*wsClient {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	clients := make([]*wsClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	return clients
}

func (s *Server) removeClients(dead []*wsClient) {
	if len(dead) > 0 {
		s.clientsMu.Lock()
		for _, c := range dead {
			delete(s.clients, c)
			if c.conn != nil {
				c.conn.Close(websocket.StatusGoingAway, "write error")
			}
		}
		s.clientsMu.Unlock()
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	setJSONSecurityHeaders(w)
	// Authenticate
	if !s.auth.Validate(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow cross-origin for TUI clients
	})
	if err != nil {
		log.Printf("websocket accept error: %v", err)
		return
	}
	conn.SetReadLimit(maxWebSocketMessageBytes)

	ctx := r.Context()
	wsc := &wsClient{conn: conn, ctx: ctx}

	s.clientsMu.Lock()
	s.clients[wsc] = struct{}{}
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, wsc)
		s.clientsMu.Unlock()
		_ = wsc.close(websocket.StatusNormalClosure, "")
	}()

	log.Printf("Remote client connected from %s", r.RemoteAddr)

	// Read messages from client and forward to Juggler
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			log.Printf("Remote client disconnected: %v", err)
			return
		}

		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}

		switch env.Type {
		case "juggler":
			// Forward to Firefox via Juggler client
			var msg juggler.Message
			if err := json.Unmarshal(env.Payload, &msg); err != nil {
				continue
			}
			var result json.RawMessage
			var err error
			if s.client == nil {
				err = fmt.Errorf("browser unavailable: server started without a kernel")
			} else {
				result, err = s.client.Call(msg.SessionID, msg.Method, msg.Params)
			}
			// Send response back
			resp := &juggler.Message{ID: msg.ID, SessionID: msg.SessionID}
			if err != nil {
				resp.Error = &juggler.Error{Message: err.Error()}
			} else {
				resp.Result = result
			}
			respData, _ := json.Marshal(resp)
			respEnv, _ := NewJugglerEnvelope(respData)
			if err := s.writeClient(wsc, respEnv); err != nil {
				log.Printf("remote response write error: %v", err)
				return
			}

		case "control":
			// Handle control commands (restart, spawn, etc.)
			s.handleControl(wsc, env.Payload)
		}
	}
}

func (s *Server) handleControl(wsc *wsClient, payload json.RawMessage) {
	var cmd struct {
		Command string          `json:"command"`
		Params  json.RawMessage `json:"params"`
		ID      int             `json:"id"`
	}
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return
	}

	respPayload := map[string]interface{}{
		"id": cmd.ID,
	}
	switch cmd.Command {
	case "ping":
		respPayload["result"] = map[string]string{"status": "pong"}
	default:
		// Dispatch to PanelAPI if available
		if s.panelAPI != nil {
			result, err := s.panelAPI.HandleMessage(cmd.Command, cmd.Params)
			if err != nil {
				respPayload["error"] = err.Error()
			} else {
				respPayload["result"] = json.RawMessage(result)
			}
		} else {
			respPayload["error"] = fmt.Sprintf("unknown command: %s", cmd.Command)
		}
	}

	env, _ := NewControlEnvelope("response", respPayload)
	if err := s.writeClient(wsc, env); err != nil {
		log.Printf("remote control write error: %v", err)
	}
}

func (s *Server) writeClient(wsc *wsClient, data []byte) error {
	baseCtx := wsc.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, s.writeTimeout)
	defer cancel()
	return wsc.write(ctx, websocket.MessageText, data)
}
