package router

import (
	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/requestctx"
)

// applyTrustedProxies declares which peers may speak for the client.
//
// Gin trusts every peer out of the box, which makes the client address the
// value the caller wrote into X-Forwarded-For — an address the request picks
// for itself. That address reaches rate limiter keys, audit records, session
// records and the access log, so the stance here is to trust nothing by
// default: an unconfigured deployment reads the address off the connection,
// which no header can rewrite.
//
// A deployment behind a load balancer or ingress names that network in
// server.trusted_proxies, read through requestctx.TrustedProxies the way the
// gRPC listener reads it, and the forwarding headers of those peers alone are
// then believed. An entry that cannot be read fails startup instead of
// quietly leaving the process on a stance nobody chose.
func applyTrustedProxies(engine *gin.Engine) error {
	proxies, err := requestctx.TrustedProxies(config.App.Server.TrustedProxies)
	if err != nil {
		return err
	}
	networks := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		networks = append(networks, proxy.String())
	}
	if err := engine.SetTrustedProxies(networks); err != nil {
		return errors.Wrap(err, "invalid server.trusted_proxies")
	}
	return nil
}
