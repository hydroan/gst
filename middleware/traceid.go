package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/hydroan/gst/types/consts"
	"github.com/hydroan/gst/util"
)

func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.Request.Header.Get(consts.TRACE_ID)
		pspanID := c.Request.Header.Get(consts.SPAN_ID)
		spanID := util.SpanID()
		if len(traceID) == 0 {
			// If traceid is empty, it means that it is the first request.
			traceID = spanID
		}
		requestID := traceID
		c.Set(consts.REQUEST_ID, requestID)
		c.Set(consts.TRACE_ID, traceID)
		c.Set(consts.PSPAN_ID, pspanID)
		c.Set(consts.SPAN_ID, spanID)
		c.Set(consts.SEQ, 0)
		c.Header(consts.HEADER_REQUEST_ID, requestID)
		c.Header(consts.HEADER_TRACE_ID, traceID)
		c.Header(consts.HEADER_SPAN_ID, spanID)
		c.Header(consts.HEADER_PSPAN_ID, pspanID)
		c.Next()
	}
}
