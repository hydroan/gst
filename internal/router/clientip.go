package router

import (
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/config"
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
// server.trusted_proxies, and the forwarding headers of those peers alone are
// then believed. An entry gin cannot parse fails startup instead of quietly
// leaving the process on a stance nobody chose.
func applyTrustedProxies(engine *gin.Engine) error {
	if err := engine.SetTrustedProxies(trustedProxies()); err != nil {
		return errors.Wrap(err, "invalid server.trusted_proxies")
	}
	return nil
}

// trustedProxies returns the configured networks with surrounding spaces
// trimmed and blank entries dropped, so that a list written as
// "10.0.0.0/8, 192.168.0.0/16" and an environment variable set to nothing both
// reach gin as something it can parse.
func trustedProxies() []string {
	configured := config.App.Server.TrustedProxies
	proxies := make([]string, 0, len(configured))
	for _, proxy := range configured {
		if proxy = strings.TrimSpace(proxy); proxy != "" {
			proxies = append(proxies, proxy)
		}
	}
	return proxies
}
