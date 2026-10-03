package main

import (
	"strings"

	"github.com/valyala/fasthttp"
)

// compressHandler wraps a handler with gzip/deflate negotiation.
//
// The frontend is served from the binary's embedded filesystem as a couple of MB
// of JavaScript and CSS, uncompressed, on every cold load. Compressing it is the
// single biggest thing available for page load time here.
//
// Two kinds of request are passed through untouched:
//
//   - Websockets. The connection is hijacked for the upgrade, and wrapping the
//     response writer breaks it. Both the agent UI and the live chat widget use
//     websockets, so this is not optional.
//   - Anything already carrying a Content-Encoding, which is compressed already.
func compressHandler(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	compressed := fasthttp.CompressHandlerBrotliLevel(h,
		fasthttp.CompressBrotliDefaultCompression,
		fasthttp.CompressDefaultCompression)

	return func(ctx *fasthttp.RequestCtx) {
		if isWebsocketUpgrade(ctx) {
			h(ctx)
			return
		}
		compressed(ctx)
	}
}

// isWebsocketUpgrade reports whether a request is a websocket handshake.
func isWebsocketUpgrade(ctx *fasthttp.RequestCtx) bool {
	return strings.EqualFold(string(ctx.Request.Header.Peek("Upgrade")), "websocket")
}
