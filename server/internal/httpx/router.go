package httpx

import (
	"net/http"
	"strings"
)

type Router interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}

type prefixRouter struct {
	router Router
	prefix string
}

func WithPrefix(router Router, prefix string) Router {
	prefix = strings.TrimSuffix(prefix, "/")
	if prefix == "" {
		prefix = "/"
	}
	return prefixRouter{router: router, prefix: prefix}
}

func (r prefixRouter) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	if pattern == "" {
		r.router.HandleFunc(r.prefix, handler)
		return
	}
	if !strings.HasPrefix(pattern, "/") {
		pattern = "/" + pattern
	}
	if r.prefix == "/" {
		r.router.HandleFunc(pattern, handler)
		return
	}
	r.router.HandleFunc(r.prefix+pattern, handler)
}
