package controller

import "github.com/hydroan/gst/internal/i18n"

// This file holds the sentences the controller answers a client with on
// either transport, and registers their translations.

// The messages of the controller's fixed refusals, each answered with the
// status named beside it.
const (
	invalidArgumentMsg = "The request contains invalid parameters."                               // 400
	notFoundMsg        = "The requested resource was not found."                                  // 404
	alreadyExistsMsg   = "The resource already exists."                                           // 409
	staleObjectMsg     = "The resource was modified by another operation. Reload and retry."      // 409
	foreignKeyMsg      = "The request refers to a record that does not exist or is still in use." // 409
)

// The descriptions of the fields the conflict of a record under a key a
// stored one holds names (see duplicatedKeyFields): the one field of a key,
// by its path, and each field of a key of several, by the paths of them all.
const (
	takenMsg         = "%s has already been taken"
	takenTogetherMsg = "the combination of %s has already been taken"
)

// The messages a batch carrying none of what its action works on is refused
// with, 400 on both transports, before any hook runs: a batch acting on
// nothing would answer success to a request that asked for nothing, which is
// how a body lost on the way to the server would read as done. Each names
// the member of the body left without an entry. The flows refuse it rather
// than a binding rule, which a project turning validation off would lift.
const (
	// missingItemsMsg answers a batch create, update or patch carrying no
	// item.
	missingItemsMsg = "items is required"
	// missingIDsMsg answers a batch delete naming no record.
	missingIDsMsg = "ids is required"
)

// The messages a call refuses a request with, where the HTTP handler's
// wording speaks of a body or a route the call has none of.
const (
	// missingIDMsg answers an item action whose message names no record.
	missingIDMsg = "id is required"
	// missingRecordMsg answers a Create, an Update or a Patch whose message
	// carries no record.
	missingRecordMsg = "record is required"
	// invalidMessageMsg answers a model or payload a validator other than
	// go-playground's refused, which names no field (see fieldViolations);
	// what failed stays in the log.
	invalidMessageMsg = "invalid request message"
)

// The messages a request body no decoder could take is refused with, 400
// each, where the refusal names no field: the body absent, of another JSON
// type than the request, no JSON at all, and refused for any other reason.
const (
	missingBodyMsg        = "request body is required"
	unexpectedJSONTypeMsg = "request body has an unexpected JSON type"
	invalidJSONMsg        = "request body is not valid JSON"
	invalidBodyMsg        = "invalid request body"
)

// The messages a request of the HTTP handlers is refused with for what it
// lacks or carries too much of.
const (
	// missingRouteParamMsg answers, with 400, a request whose configured
	// route parameter is absent.
	missingRouteParamMsg = "route parameter is required"
	// missingUploadFileMsg answers an import request whose multipart form
	// carries no "file" field, keeping the multipart reader's own error text
	// out of the response for the same reason bind failures render stable
	// messages.
	missingUploadFileMsg = "upload file is required"
	// tooLargeFileMsg answers, with 400, an upload over maxImportSize.
	tooLargeFileMsg = "file too large"
)

// The messages a Stream action ends with before it served anything.
const (
	// unimplementedMsg answers a Stream action nothing serves, each
	// transport with its own status (see unserved).
	unimplementedMsg = "the Stream action is served by no Stream method of its kind"
	// firstMessageMsg refuses a stream the client ended before its first
	// message (see FirstMessage).
	firstMessageMsg = "the stream ended before its first message, which carries the route parameters"
	// requestTimeoutMsg closes, as a 408, a server stream whose client did
	// not send its request in time (see awaitRequest).
	requestTimeoutMsg = "the request of the stream did not arrive in time"
)

// failedCheckMsg words the failure of a rule the validator has no sentence
// for in the language the process answers in, the field and then the rule:
// "address.zip failed the hostname check" (see fieldViolations).
const failedCheckMsg = "%s failed the %s check"

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		invalidArgumentMsg: "请求包含无效的参数。",
		notFoundMsg:        "请求的资源不存在。",
		alreadyExistsMsg:   "资源已存在。",
		staleObjectMsg:     "资源已被其他操作修改，请刷新后重试。",
		foreignKeyMsg:      "请求引用的记录不存在，或仍在使用中。",

		takenMsg:         "%s已被使用",
		takenTogetherMsg: "%s的组合已被使用",

		missingItemsMsg: "items为必填字段",
		missingIDsMsg:   "ids为必填字段",

		missingIDMsg:      "id为必填字段",
		missingRecordMsg:  "record为必填字段",
		invalidMessageMsg: "请求消息无效",

		missingBodyMsg:        "请求体不能为空",
		unexpectedJSONTypeMsg: "请求体的 JSON 类型不符合预期",
		invalidJSONMsg:        "请求体不是合法的 JSON",
		invalidBodyMsg:        "请求体无效",

		missingRouteParamMsg: "缺少路由参数",
		missingUploadFileMsg: "缺少上传的文件",
		tooLargeFileMsg:      "文件过大",

		unimplementedMsg:  "没有对应类型的 Stream 方法为该流式接口提供服务",
		firstMessageMsg:   "流在发出第一条消息之前就结束了，路由参数由这条消息携带",
		requestTimeoutMsg: "流的请求没有及时到达",

		failedCheckMsg: "%s未通过%s校验",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		invalidArgumentMsg: "請求包含無效的參數。",
		notFoundMsg:        "請求的資源不存在。",
		alreadyExistsMsg:   "資源已存在。",
		staleObjectMsg:     "資源已被其他操作修改，請重新整理後再試一次。",
		foreignKeyMsg:      "請求參照的記錄不存在，或仍在使用中。",

		takenMsg:         "%s已被使用",
		takenTogetherMsg: "%s的組合已被使用",

		missingItemsMsg: "items為必填欄位",
		missingIDsMsg:   "ids為必填欄位",

		missingIDMsg:      "id為必填欄位",
		missingRecordMsg:  "record為必填欄位",
		invalidMessageMsg: "請求訊息無效",

		missingBodyMsg:        "請求主體不可為空",
		unexpectedJSONTypeMsg: "請求主體的 JSON 型別不符合預期",
		invalidJSONMsg:        "請求主體不是有效的 JSON",
		invalidBodyMsg:        "請求主體無效",

		missingRouteParamMsg: "缺少路由參數",
		missingUploadFileMsg: "缺少上傳的檔案",
		tooLargeFileMsg:      "檔案過大",

		unimplementedMsg:  "沒有對應類型的 Stream 方法為這個串流介面提供服務",
		firstMessageMsg:   "串流在送出第一則訊息之前就結束了，路由參數由這則訊息攜帶",
		requestTimeoutMsg: "串流的請求沒有及時送達",

		failedCheckMsg: "%s未通過%s驗證",
	})
}
