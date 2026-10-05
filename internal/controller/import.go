package controller

import (
	"bytes"
	"context"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/types"
	"github.com/hydroan/gst/logger"
	gstotel "github.com/hydroan/gst/otel"
	"go.uber.org/zap"
)

// MaxImportSize is the largest upload an import accepts, 5 MiB.
const MaxImportSize = 5 * 1024 * 1024

// tooLargeFileMsg answers, with 400, an upload over MaxImportSize.
const tooLargeFileMsg = "too large file"

// missingUploadFileMsg answers an import request whose multipart form carries
// no "file" field, keeping the multipart reader's own error text out of the
// response for the same reason bind failures render stable messages.
const missingUploadFileMsg = "upload file is required"

// ImportHandler returns a Gin handler that imports resources from an uploaded file.
//
// The handler reads the multipart form file named "file", rejects files larger
// than MaxImportSize, passes the file content to the phase service's Import
// method, and fills creator/updater fields on the returned models. Rows are
// then written by explicit intent instead of an upsert: a row carrying an ID
// replaces that existing record (missing IDs fail with 404), and a row without
// an ID is created (unique-key collisions fail with 409). Both writes share
// one transaction, so an import is all-or-nothing.
func ImportHandler[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	a := newAction[M, REQ, RSP](routeFromConfig(cfg...), consts.Import)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := a.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.Import)
		// NOTE: the form field name is "file", it must be agreed on with the frontend.
		file, err := c.FormFile("file")
		if err != nil {
			log.Errorz("read upload file failed", zap.Error(err))
			response.Error(c, badRequest(missingUploadFileMsg))
			gstotel.RecordError(span, err)
			return
		}
		// check file size.
		if file.Size > int64(MaxImportSize) {
			log.Errorz(tooLargeFileMsg)
			response.Error(c, badRequest(tooLargeFileMsg))
			gstotel.RecordError(span, errors.New(tooLargeFileMsg))
			return
		}
		fd, err := file.Open()
		if err != nil {
			log.Errorz("read upload file failed", zap.Error(err))
			response.Error(c, err)
			gstotel.RecordError(span, err)
			return
		}
		defer fd.Close()

		buf := new(bytes.Buffer)
		if _, err = io.Copy(buf, fd); err != nil {
			log.Errorz("read upload file failed", zap.Error(err))
			response.Error(c, err)
			gstotel.RecordError(span, err)
			return
		}
		ml, err := traceServiceCall(ctrlSpanCtx, a.serviceSpan(consts.Import), a.name, func(spanCtx context.Context) ([]M, error) {
			return a.service().
				Import(types.NewServiceContext(c, spanCtx, consts.Import), buf)
		})
		if err != nil {
			log.Errorz("service operation failed", zap.Error(err))
			response.Error(c, err)
			gstotel.RecordError(span, err)
			return
		}

		// The service's Import only parses the file into models; the controller
		// owns persistence. Stamp the audit fields, then split rows by intent:
		// an ID marks a replacement of that record, no ID marks a creation.
		toCreate := make([]M, 0, len(ml))
		toUpdate := make([]M, 0, len(ml))
		for i := range ml {
			ml[i].SetCreatedBy(c.GetString(consts.CTX_USERNAME))
			ml[i].SetUpdatedBy(c.GetString(consts.CTX_USERNAME))
			if len(ml[i].GetID()) > 0 {
				toUpdate = append(toUpdate, ml[i])
			} else {
				toCreate = append(toCreate, ml[i])
			}
		}
		// One transaction for the whole import: a duplicate on the create side
		// or a missing ID on the update side rolls everything back.
		//
		// TODO: the controller opens this transaction, and with it decides
		// how an import persists: rows with an id replace their records, the
		// rest are created, and all of it rolls back together. Decide whether
		// the service's Import should persist the rows and make that choice.
		if err := database.Transaction(requestContext(c), func(txCtx context.Context) error {
			if err := database.Database[M](txCtx).Create(toCreate...); err != nil {
				return err
			}
			return database.Database[M](txCtx).Update(toUpdate...)
		}); err != nil {
			log.Errorz("database operation failed", zap.Error(err))
			response.Error(c, databaseError(err))
			gstotel.RecordError(span, err)
			return
		}
		response.JSON(c)
	}
}
