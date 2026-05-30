package srv

import (
	"context"
	"net/http"
)

type Service interface {
	RegisterMux(ctx context.Context, mx *http.ServeMux)
}
