package mcpserver

import (
	"errors"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kanthorlabs/sag/internal/inventory"
)

type Options struct {
	Inventory *inventory.Inventory
	Version   string
}

func New(opts Options) (*mcp.Server, error) {
	return nil, errors.ErrUnsupported
}

func Handler(srv *mcp.Server) http.Handler {
	return nil
}
