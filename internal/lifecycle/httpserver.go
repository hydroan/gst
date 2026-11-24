package lifecycle

import (
	"context"
	"net/http"

	"go.uber.org/zap"
)

// ShutdownHTTPServer shuts server down within ctx: it stops accepting
// connections and waits for the requests in flight until ctx ends, then
// closes the connections that drain left open, their requests cut off,
// rather than leave them to run past the teardown of what they use. name
// leads the entries it logs, "pprof server" for one, and their caller is the
// line that called ShutdownHTTPServer, so the entries read as the server's
// own.
func ShutdownHTTPServer(ctx context.Context, server *http.Server, name string) {
	log := zap.S().WithOptions(zap.AddCallerSkip(1))
	log.Infow(name + " shutdown initiated")
	if err := server.Shutdown(ctx); err != nil {
		log.Warnw(name+" closing the connections its drain left open", "err", err, "reason", context.Cause(ctx))
		if closeErr := server.Close(); closeErr != nil {
			log.Errorw(name+" close failed", "err", closeErr)
		}
	} else {
		log.Infow(name + " shutdown completed")
	}
}
