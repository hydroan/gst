package controller

import (
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// PatchManyHandler returns a Gin handler that partially updates multiple
// resources.
//
// When M, REQ, and RSP are the same type, the handler binds the JSON body into
// batch[M] along with the set of fields each item carried, and runs the
// batch patch flow (see patchManyFlow), which answers with the patched
// records.
//
// When REQ or RSP differs from M, the handler is the phase service's (see
// serviceHandler): its PatchMany method runs on the bound payload.
func PatchManyHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), apiaction.PatchMany, apiaction.PatchManyBefore, apiaction.PatchManyAfter)
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

		log := logger.Controller.WithContext(c.Request.Context(), apiaction.PatchMany)

		var req batch[M]
		body, err := readJSONRequestBody(c)
		if err != nil {
			response.Error(c, reportFailure(log, span, "bind request body failed", err, invalidArgument(err)))
			return
		}
		fieldSets, fieldErr := patchManyFieldSetsFromJSONBody(a.typ, body)
		if fieldErr != nil && !errors.Is(fieldErr, io.EOF) {
			response.Error(c, reportFailure(log, span, "bind request body failed", fieldErr, invalidArgument(fieldErr)))
			return
		}
		if reqErr := decodeJSONRequest(c, &req); reqErr != nil && !errors.Is(reqErr, io.EOF) {
			response.Error(c, reportFailure(log, span, "bind request body failed", reqErr, invalidArgument(reqErr)))
			return
		}
		normalizeBatch(&req)
		// A versioned model must carry a version on every item, exactly like
		// the single-resource patch; failing the whole batch up front keeps
		// the all-or-nothing shape a defective request deserves. See
		// modelschema.Version. Each item is then validated on the fields it
		// names, like the single-resource patch too. Every version is checked
		// before any item is validated, the order the call checks a batch in,
		// so a batch defective in several items is refused for the same one
		// on both transports.
		versionField, versioned := modelschema.VersionFieldName(a.newModel())
		fieldsOf := func(i int) patchFieldSet {
			if i < len(fieldSets) {
				return fieldSets[i]
			}
			return patchFieldSet{}
		}
		for i := range req.Items {
			if _, ok := fieldsOf(i)[versionField]; versioned && !ok {
				response.Error(c, reportFailure(log, span, "versioned model patched without its version", nil, databaseError(database.ErrVersionRequired),
					zap.Int("item", i), zap.String("field", versionField)))
				return
			}
		}
		for i, item := range req.Items {
			if fieldErr := validatePatchFields(item, fieldsOf(i)); fieldErr != nil {
				fieldErr = clientSafeItemBindError(i, fieldErr)
				response.Error(c, reportFailure(log, span, "bind request body failed", fieldErr, invalidArgument(fieldErr)))
				return
			}
		}

		rsp, err := a.patchManyFlow(requestContext(c), ginServiceContext(c), &req, fieldSets)
		if err != nil {
			response.Error(c, err)
			return
		}
		response.JSON(c, rsp)
	}
}
