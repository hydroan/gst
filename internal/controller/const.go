package controller

const (
	MAX_AVATAR_SIZE = 1024 * 1024 * 2   //nolint:staticcheck // 2M
	MAX_IMPORT_SIZE = 5 * 1024 * 1024   //nolint:staticcheck // 5M
	MAX_UPLOAD_SIZE = 1024 * 1024 * 100 //nolint:staticcheck // 100M
)

// tooLargeFileMsg answers an upload over MAX_IMPORT_SIZE. It is carried as a
// message under CodeInvalidParam rather than as a code of its own, for the same
// reason as missingRouteParamMsg.
const tooLargeFileMsg = "too large file"

const (
	TOKEN         = "token"
	ACCESS_TOKEN  = "access_token"  //nolint:staticcheck
	REFRESH_TOKEN = "refresh_token" //nolint:staticcheck
	NAME          = "name"          //nolint:staticcheck
	ID            = "id"
	SESSION_ID    = "session_id" //nolint:staticcheck
)
