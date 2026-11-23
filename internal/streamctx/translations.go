package streamctx

import "github.com/hydroan/gst/internal/i18n"

// ShutdownMsg is what the client of a stream the shutdown ended is told, the
// same sentence on both transports and in every form such a stream ends in:
// the message of the 503 a Server-Sent Events stream that had not opened is
// answered with, the reason a WebSocket connection is closed with, and the
// message of the Unavailable status a gRPC stream is answered with.
const ShutdownMsg = "the server is shutting down"

// init registers the translations of the sentence above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		ShutdownMsg: "服务器正在关闭",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		ShutdownMsg: "伺服器正在關閉",
	})
}
