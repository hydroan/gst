package controller

import (
	"bytes"
	"context"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/database"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/logger"
	"github.com/hydroan/gst/pkg/filetype"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// Import handles an import request with the default factory settings.
func Import[M types.Model, REQ types.Request, RSP types.Response](c *gin.Context) {
	ImportFactory[M, REQ, RSP]()(c)
}

// ImportFactory returns a Gin handler that imports resources from an uploaded file.
//
// The handler reads the multipart form file named "file", rejects files larger
// than MAX_IMPORT_SIZE, passes the file content to the phase service's Import
// method, and fills creator/updater fields on the returned models. Rows are
// then written by explicit intent instead of an upsert: a row carrying an ID
// replaces that existing record (missing IDs fail with 404), and a row without
// an ID is created (unique-key collisions fail with 409). Both writes share
// one transaction, so an import is all-or-nothing.
func ImportFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	meta := newFactoryMeta[M, REQ, RSP](routeFromConfig(cfg...), consts.PHASE_IMPORT)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := meta.startControllerSpan(c)
		defer span.End()

		log := logger.Controller.WithContext(c.Request.Context(), consts.PHASE_IMPORT)
		// NOTE:字段为 file 必须和前端协商好.
		file, err := c.FormFile("file")
		if err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// check file size.
		if file.Size > int64(MAX_IMPORT_SIZE) {
			log.Error(CodeTooLargeFile)
			JSON(c, CodeTooLargeFile)
			gstotel.RecordError(span, errors.New(CodeTooLargeFile.Msg()))
			return
		}
		fd, err := file.Open()
		if err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		defer fd.Close()

		buf := new(bytes.Buffer)
		if _, err = io.Copy(buf, fd); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}
		// filetype must be png or jpg.
		filetype, mime := filetype.DetectBytes(buf.Bytes())
		_, _ = filetype, mime

		// check filetype

		ml, err := meta.traceServiceImport(ctrlSpanCtx, consts.PHASE_IMPORT, func(spanCtx context.Context) ([]M, error) {
			return meta.service().
				Import(types.NewServiceContext(c, spanCtx, consts.PHASE_IMPORT), buf)
		})
		if err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
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
		if err := database.Transaction(requestContext(c), func(txCtx context.Context) error {
			if err := handler(txCtx).Create(toCreate...); err != nil {
				return err
			}
			return handler(txCtx).Update(toUpdate...)
		}); err != nil {
			log.Error(err)
			JSON(c, writeErrorCoder(err))
			gstotel.RecordError(span, err)
			return
		}
		// // record operation log to database.
		// typ := reflect.TypeOf(*new(M)).Elem()
		// var tableName string
		// items := strings.Split(typ.Name(), ".")
		// if len(items) > 0 {
		// 	tableName = pluralizeCli.Plural(strings.ToLower(items[len(items)-1]))
		// }
		// record, _ := json.Marshal(ml)
		// if err := database.Database[*model.OperationLog]().WithDB(db).Create(&model.OperationLog{
		// 	Op:        model.OperationTypeImport,
		// 	Model:     typ.Name(),
		// 	Table:     tableName,
		// 	Record:    util.BytesToString(record),
		// 	IP:        c.ClientIP(),
		// 	User:      c.GetString(consts.CTX_USERNAME),
		// 	TraceID: c.GetString(consts.TRACE_ID),
		// 	URI:       c.Request.RequestURI,
		// 	Method:    c.Request.Method,
		// 	UserAgent: c.Request.UserAgent(),
		// }); err != nil {
		// 	log.Error("failed to write operation log to database: ", err.Error())
		// }
		JSON(c, CodeSuccess)
	}
}
