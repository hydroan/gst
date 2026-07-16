package controller

const (
	MAX_AVATAR_SIZE = 1024 * 1024 * 2   //nolint:staticcheck // 2M
	MAX_IMPORT_SIZE = 5 * 1024 * 1024   //nolint:staticcheck // 5M
	MAX_UPLOAD_SIZE = 1024 * 1024 * 100 //nolint:staticcheck // 100M
)

const (
	TOKEN         = "token"
	ACCESS_TOKEN  = "access_token"  //nolint:staticcheck
	REFRESH_TOKEN = "refresh_token" //nolint:staticcheck
	NAME          = "name"          //nolint:staticcheck
	ID            = "id"
	SESSION_ID    = "session_id" //nolint:staticcheck
)

// defaultLimit is the full-table safety bottom line for list queries whose
// model exposes no client-adjustable page size.
const defaultLimit = 1000

// defaultPageSize and maxPageSize bound the _size parameter on models that
// embed Pagination or Cursor: an unset size defaults to a small first page
// and oversized values clamp to the cap instead of erroring, matching common
// API practice (bulk retrieval belongs to the Export action).
const (
	defaultPageSize = 20
	maxPageSize     = 100
)
