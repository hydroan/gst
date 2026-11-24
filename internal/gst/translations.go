package gst

import "github.com/hydroan/gst/internal/i18n"

// FailureMsg is the message a failure carrying no Error is answered with on
// every transport, the server's own failure: 500 with it in the HTTP
// envelope (see response.Error), Internal with it over gRPC (see
// grpcserver.StatusError). What went wrong stays in the log.
const FailureMsg = "The server could not process the request."

// init registers the translations of the sentence above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		FailureMsg: "服务器无法处理该请求。",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		FailureMsg: "伺服器無法處理這個請求。",
	})
}
