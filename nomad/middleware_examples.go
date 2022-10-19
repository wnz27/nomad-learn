package nomad

import (
	"fmt"
	"time"
)

func forwardingMiddleware[Req RPCRequest, Resp any](method string) MiddlewareFunc[Req, Resp] {
	return func(
		ctx *RPCHandlerContext,
		args Req,
		reply Resp,
		handler RPCHandlerFunc[Req, Resp],
	) error {
		fmt.Println("forwarding: ", method)
		return handler(ctx, args, reply)
	}
}

func forwardingAuthoritativeRegionMiddleware[Req RPCRequest, Resp any](method, region string) MiddlewareFunc[Req, Resp] {
	return func(
		ctx *RPCHandlerContext,
		args Req,
		reply Resp,
		handler RPCHandlerFunc[Req, Resp],
	) error {
		args.SetRegion(region)
		fmt.Println("forwarding:", method, region)
		return handler(ctx, args, reply)
	}
}

func authenticationMiddleware[Req RPCRequest, Resp any]() MiddlewareFunc[Req, Resp] {
	return func(
		ctx *RPCHandlerContext,
		args Req,
		reply Resp,
		handler RPCHandlerFunc[Req, Resp],
	) error {
		fmt.Printf("authentication: verifying token %q\n", args.RequestToken())
		return handler(ctx, args, reply)
	}
}

func authorizationMiddleware[Req RPCRequest, Resp any](perm string) MiddlewareFunc[Req, Resp] {
	return func(
		ctx *RPCHandlerContext,
		args Req,
		reply Resp,
		handler RPCHandlerFunc[Req, Resp],
	) error {
		fmt.Printf("authorization: verifying token %q has permission %q\n", args.RequestToken(), perm)
		return handler(ctx, args, reply)
	}
}

func metricsMiddleware[Req RPCRequest, Resp any](labels []string) MiddlewareFunc[Req, Resp] {
	return func(
		ctx *RPCHandlerContext,
		args Req,
		reply Resp,
		handler RPCHandlerFunc[Req, Resp],
	) error {
		defer fmt.Println("metrics:", labels, time.Now().UTC())
		return handler(ctx, args, reply)
	}
}
