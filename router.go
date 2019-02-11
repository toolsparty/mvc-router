package router

import (
	"log"
	"net/http"
	"regexp"

	"github.com/buaazp/fasthttprouter"
	"github.com/pkg/errors"
	"github.com/toolsparty/mvc"
	"github.com/valyala/fasthttp"
	"runtime"
	"github.com/getsentry/raven-go"
	"fmt"
)

type HandleFunc func(path string, handle fasthttp.RequestHandler)
type Middleware func(*fasthttp.RequestCtx, mvc.Action) (mvc.Action, error)

// Router implements interface mvc.Router
type Router struct {
	// fasthttp router instance
	router *fasthttprouter.Router

	// list of middleware
	middleware []Middleware

	// mvc application instance
	app *mvc.App
}

func (s *Router) Route(app *mvc.App) error {
	s.app = app

	actions := app.Actions()

	s.router = fasthttprouter.New()
	re := regexp.MustCompile(`^(\w{3,7})+\s(.*)`)

	for addr, action := range actions {
		action := action
		res := re.FindAllStringSubmatch(addr, -1)

		var path, method string
		if len(res) >= 1 && len(res[0]) >= 2 {
			method = res[0][1]
			path = res[0][len(res[0])-1]
		} else {
			return errors.New(addr + " is invalid path")
		}

		var handle HandleFunc

		switch method {
		case http.MethodPost:
			handle = s.router.POST
		case http.MethodPut:
			handle = s.router.PUT
		case http.MethodPatch:
			handle = s.router.PATCH
		case http.MethodDelete:
			handle = s.router.DELETE
		case http.MethodHead:
			handle = s.router.HEAD
		case http.MethodOptions:
			handle = s.router.OPTIONS
		case http.MethodGet:
			fallthrough
		default:
			handle = s.router.GET
		}

		handle(path, s.Handle(action))
	}

	addr := app.Config().GetString("http.host") + ":" + app.Config().GetString("http.port")
	log.Println("Listen on", addr)

	return fasthttp.ListenAndServe(addr, s.router.Handler)
}

// Handle request by mvc.Action and apply middleware
func (s *Router) Handle(action mvc.Action) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		defer func() {
			if rec := recover(); rec != nil {
				s := make([]byte, 4<<10)
				l := runtime.Stack(s, false)

				err := fmt.Errorf("recovered from %v with stack: %s info: %v", rec, s[0:l], info)
				raven.CaptureError(err, nil)
				log.Println(err)
			}
		}()

		var err error
		var fh mvc.Action

		defer func() {
			s.handleError(ctx, err)
		}()

		for _, mw := range s.middleware {
			fh, err = mw(ctx, action)
			if err != nil {
				return
			}
		}

		if fh == nil {
			err = action(ctx)
		} else {
			err = fh(ctx)
		}
	}
}

// Middleware adding middleware
func (s *Router) Middleware(mw Middleware) {
	s.middleware = append(s.middleware, mw)
}

// handleError handling errors
func (s *Router) handleError(ctx *fasthttp.RequestCtx, e error) {
	if e == nil {
		return
	}

	defer s.app.Log(e)

	view, ok := s.app.View("error").(mvc.View)
	if !ok {
		s.app.Log("error view not found")
		return
	}

	err := view.Render(ctx, "error", mvc.ViewParams{
		"error": e,
	})
	if err != nil {
		s.app.Log(err)
	}
}
