package controller

import (
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/logger"
	"github.com/hydroan/gst/internal/response"
	"go.uber.org/zap"
)

// PatchHandler returns a Gin handler that partially updates one resource.
//
// When M, REQ, and RSP are the same type, the handler reads the resource id
// from the configured route parameter (the id carried by the body is ignored),
// binds the JSON body into M along with the set of fields the body carried,
// and runs the patch flow (see patchFlow), which answers with the patched
// record.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its Patch method runs on the bound payload.
func PatchHandler[M gst.Model, REQ gst.Request, RSP gst.Response](cfg ...*gst.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.Patch, apiaction.PatchBefore, apiaction.PatchAfter)
	if !a.typesEqual {
		return a.serviceHandler()
	}
	return func(c *gin.Context) {
		// The span context lives on in the request context the span start
		// rebinds, which requestContext reads for the flow.
		_, span, ok := a.beginRequest(c)
		defer span.End()
		if !ok {
			return
		}

		log := logger.Controller.WithContext(c.Request.Context(), apiaction.Patch)

		req := a.newModel()
		body, err := readJSONRequestBody(c)
		if err != nil {
			response.Error(c, reportFailure(log, span, "bind request body failed", err, invalidArgument(err)))
			return
		}
		fields, err := patchFieldSetFromJSONBody(a.typ, body)
		if err != nil && !errors.Is(err, io.EOF) {
			response.Error(c, reportFailure(log, span, "bind request body failed", err, invalidArgument(err)))
			return
		}
		if err = decodeJSONRequest(c, &req); err != nil {
			// A single-resource patch without a body patches nothing; refuse it
			// with a stable message instead of the bare io.EOF text.
			err = requiredBodyError(err)
			response.Error(c, reportFailure(log, span, "bind request body failed", err, invalidArgument(err)))
			return
		}
		// A versioned model is patched with its version (see versionOmitted).
		// Checked once the body is read, so that a request carrying no record
		// is refused for the record it lacks, the way a call is, and before
		// the existence query, so that a defective request costs no database
		// work.
		if versionField, omitted := versionOmitted(req, fields); omitted {
			response.Error(c, reportFailure(log, span, "versioned model patched without its version", nil, databaseError(database.ErrVersionRequired), zap.String("field", versionField)))
			return
		}
		a.normalizeModel(&req)
		if err = validatePatchFields(req, fields); err != nil {
			err = clientSafeBindError(err)
			response.Error(c, reportFailure(log, span, "bind request body failed", err, invalidArgument(err)))
			return
		}
		id, ok := routeID(c, cfg, log, span)
		if !ok {
			return
		}

		cur, err := a.patchFlow(requestContext(c), ginServiceContext(c), id, req, fields)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, cur)
	}
}
