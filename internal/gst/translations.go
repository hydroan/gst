package gst

import "github.com/hydroan/gst/internal/i18n"

// FailureMsg is the message a failure carrying no Error is answered with on
// every transport, the server's own failure: 500 with it in the HTTP
// envelope (see response.Error), Internal with it over gRPC (see
// grpcserver.StatusError). What went wrong stays in the log.
const FailureMsg = "The server could not process the request."

// TimeoutMsg is the message a request not answered within the time the
// server gives one, server.write_timeout, is answered with on every
// transport: 504 with it in the HTTP envelope, by the deadline of the
// built-in middleware chain, and DeadlineExceeded with it over gRPC, by the
// deadline of the server's interceptor chain.
const TimeoutMsg = "request timeout"

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		FailureMsg: "服务器无法处理该请求。",
		TimeoutMsg: "请求超时",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		FailureMsg: "伺服器無法處理這個請求。",
		TimeoutMsg: "請求逾時",
	})
}
