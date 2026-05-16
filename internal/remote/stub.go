package remote

import (
	"context"

	"vulpineos/internal/juggler"
)

type ContextRegistry struct{}

func NewContextRegistry() *ContextRegistry {
	return &ContextRegistry{}
}

type Context struct {
	BrowserContextID string
}

func (c *ContextRegistry) Get(id string) (*Context, error) {
	return nil, nil
}

func (c *ContextRegistry) Set(id string, ctx *Context) {
}

func (c *ContextRegistry) Delete(id string) {
}

type Client struct{}

func Dial(ctx context.Context, addr, apiKey string) (*Client, error) {
	return &Client{}, nil
}

func (c *Client) Close() {}

type Server struct{}

func NewServer(addr string, apiKey string, client *juggler.Client) *Server {
	return &Server{}
}

func (s *Server) Serve() error {
	return nil
}

func (s *Server) Close() error {
	return nil
}

func (s *Server) Addr() string {
	return ""
}

func (s *Server) BroadcastEvent(eventType string, agentID string, payload []byte) {}

type PanelAPI struct{}

func ServePanel(mux interface{}, fs interface{}) {}

func (p *PanelAPI) HandleMessage(msgType string, agentID string, payload interface{}) ([]byte, error) {
	return nil, nil
}