package controller

const MAX_IMPORT_SIZE = 5 * 1024 * 1024 //nolint:staticcheck // 5M

// tooLargeFileMsg answers an upload over MAX_IMPORT_SIZE. It is carried as a
// message under CodeInvalidParam rather than as a code of its own, for the same
// reason as missingRouteParamMsg.
const tooLargeFileMsg = "too large file"

// missingUploadFileMsg answers an import request whose multipart form carries
// no "file" field, keeping the multipart reader's own error text out of the
// response for the same reason bind failures render stable messages.
const missingUploadFileMsg = "upload file is required"
