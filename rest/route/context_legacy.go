// +build !go1.7

package route

import (
	"context"
	"net/http"
)

func getContext(r *http.Request) context.Context { return context.Background() }
