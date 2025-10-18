package middleware

import "net/http"

type Chain struct {
	handlers []func(http.Handler) http.Handler
}

func NewChain() *Chain {
	return &Chain{handlers: []func(http.Handler) http.Handler{}}
}

func (a *Chain) Add(handlers ...func(http.Handler) http.Handler) {
	a.handlers = append(a.handlers, handlers...)
}

func (a *Chain) Apply(h http.Handler) http.Handler {
	for i := len(a.handlers) - 1; i >= 0; i-- {
		h = a.handlers[i](h)
	}
	return h
}
