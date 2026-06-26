package controller

import (
	"bytes"
	"context"
	"io"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	. "github.com/hydroan/gst/internal/response"
	"github.com/hydroan/gst/internal/serviceregistry"
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
// method, fills creator/updater fields on the returned models, updates those
// models through the configured database handler, and returns a success
func ImportFactory[M types.Model, REQ types.Request, RSP types.Response](cfg ...*types.ControllerConfig[M]) gin.HandlerFunc {
	handler, _ := extractConfig(cfg...)
	return func(c *gin.Context) {
		ctrlSpanCtx, span := startControllerSpan[M](c, consts.PHASE_IMPORT)
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

		ml, err := traceServiceImport(ctrlSpanCtx, consts.PHASE_IMPORT, func(spanCtx context.Context) ([]M, error) {
			return serviceregistry.Resolve[M, REQ, RSP](consts.PHASE_IMPORT).
				Import(types.NewServiceContext(c, spanCtx, consts.PHASE_IMPORT), buf)
		})
		if err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
			gstotel.RecordError(span, err)
			return
		}

		// service layer already create/update the records in database, just update fields "created_by", "updated_by".
		for i := range ml {
			ml[i].SetCreatedBy(c.GetString(consts.CTX_USERNAME))
			ml[i].SetUpdatedBy(c.GetString(consts.CTX_USERNAME))
		}
		if err := handler(requestContext(c)).Update(ml...); err != nil {
			log.Error(err)
			JSON(c, CodeFailure.WithErr(err))
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
