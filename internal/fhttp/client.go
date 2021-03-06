package fhttp

import (
	"time"

	"github.com/geekypanda/httpcache/internal"
	"github.com/valyala/fasthttp"
)

// ClientHandler is the client-side handler
// for each of the cached route paths's response
// register one client handler per route.
//
// it's just calls a remote cache service server/handler,
//  which lives on other, external machine.
//
type ClientHandler struct {
	// bodyHandler the original route's handler
	bodyHandler fasthttp.RequestHandler

	life time.Duration

	remoteHandlerURL string
}

// NewClientHandler returns a new remote client handler
// which asks the remote handler the cached entry's response
// with a GET request, or add a response with POST request
// these all are done automatically, users can use this
// handler as they use the local.go/NewHandler
//
// the ClientHandler is useful when user
// wants to apply horizontal scaling to the app and
// has a central http server which handles
func NewClientHandler(bodyHandler fasthttp.RequestHandler, life time.Duration, remote string) *ClientHandler {
	return &ClientHandler{
		bodyHandler:      bodyHandler,
		life:             life,
		remoteHandlerURL: remote,
	}
}

// ClientFasthttp is used inside the global RequestFasthttp function
// this client is an exported variable because the maybe the remote cache service is running behind ssl,
// in that case you are able to set a Transport inside it
var ClientFasthttp = &fasthttp.Client{WriteTimeout: internal.RequestCacheTimeout, ReadTimeout: internal.RequestCacheTimeout}

var (
	methodGetBytes  = []byte("GET")
	methodPostBytes = []byte("POST")
)

// ServeHTTP , or remote cache client whatever you like, it's the client-side function of the ServeHTTP
// sends a request to the server-side remote cache Service and sends the cached response to the frontend client
// it is used only when you achieved something like horizontal scaling (separate machines)
// look ../remote/remote.ServeHTTP for more
//
// if cache din't find then it sends a POST request and save the bodyHandler's body to the remote cache.
//
// It takes 3 parameters
// the first is the remote address (it's the address you started your http server which handled by the Service.ServeHTTP)
// the second is the handler (or the mux) you want to cache
// and the  third is the, optionally, cache expiration,
// which is used to set cache duration of this specific cache entry to the remote cache service
// if <=minimumAllowedCacheDuration then the server will try to parse from "cache-control" header
//
// client-side function
func (h *ClientHandler) ServeHTTP(reqCtx *fasthttp.RequestCtx) {
	uri := &internal.URIBuilder{}
	uri.ServerAddr(h.remoteHandlerURL).ClientURI(string(reqCtx.URI().RequestURI())).ClientMethod(string(reqCtx.Method()))

	req := fasthttp.AcquireRequest()

	req.URI().Update(uri.String())
	req.Header.SetMethodBytes(methodGetBytes)

	res := fasthttp.AcquireResponse()
	// println("[FASTHTTP] GET Do to the remote cache service with the url: " + req.URI().String())

	err := ClientFasthttp.Do(req, res)
	if err != nil || res.StatusCode() == internal.FailStatus {

		//	println("lets execute the main fasthttp handler times: ")
		//	print(times)
		//		times++
		// if not found on cache, then execute the handler and save the cache to the remote server
		h.bodyHandler(reqCtx)
		// save to the remote cache

		body := reqCtx.Response.Body()[0:]
		if len(body) == 0 {
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(res)
			return // do nothing..
		}
		req.Reset()

		uri.StatusCode(reqCtx.Response.StatusCode())
		uri.Lifetime(h.life)
		uri.ContentType(string(reqCtx.Response.Header.Peek(internal.ContentTypeHeader)))

		req.URI().Update(uri.String())
		req.Header.SetMethodBytes(methodPostBytes)
		req.SetBody(body)

		//	go func() {
		//	println("[FASTHTTP] POST Do to the remote cache service with the url: " + req.URI().String() + " , method validation: " + string(req.Header.Method()))
		//	err := ClientFasthttp.Do(req, res)
		//	if err != nil {
		//	println("[FASTHTTP] ERROR WHEN POSTING TO SAVE THE CACHE ENTRY. TRACE: " + err.Error())
		//	}
		ClientFasthttp.Do(req, res)
		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(res)
		//	}()

	} else {
		// get the status code , content type and the write the response body
		statusCode := res.StatusCode()
		//	println("[FASTHTTP] ServeHTTP: WRITE WITH THE CACHED, StatusCode: ", statusCode)
		cType := res.Header.ContentType()
		reqCtx.SetStatusCode(statusCode)
		reqCtx.Response.Header.SetContentTypeBytes(cType)

		reqCtx.Write(res.Body())

		fasthttp.ReleaseRequest(req)
		fasthttp.ReleaseResponse(res)
	}

}
